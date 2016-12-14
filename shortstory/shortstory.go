// Copyright 2016 Adam H. Leventhal. All rights reserved.
// Licensed under the Apache License, version 2.0:
// http://www.apache.org/licenses/LICENSE-2.0

package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"sort"
	"time"

	"github.com/attic-labs/noms/go/datas"
	//"github.com/attic-labs/noms/go/hash"
	"github.com/attic-labs/noms/go/spec"
	"github.com/attic-labs/noms/go/types"
	"github.com/garyburd/redigo/redis"
)

// Turn the items into threads:
// Map<Number, Struct Story {
//	id Number
//	time Number
//
//	// Optional
//	deleted, dead Bool
//	descendants, score Number
//	text, url, title, by String
//
//	comments List<Struct Comment {
//		id Number
//		time Number
//
//		// Optional
//		deleted, dead Bool
//		text, by String
//
//		comments List<Cycle<0>>
//	}>
// }>
//

var nothing types.Value
var nothingType *types.Type

func init() {
	nothing = types.NewStruct("Nothing", types.StructData{})
	nothingType = nothing.Type()
}

var commentType *StructType
var storyType *StructType

func main() {
	flag.Usage = func() {
		fmt.Printf("Usage: %s <src> <dst>\n", path.Base(os.Args[0]))
	}
	flag.Parse()
	if flag.NArg() != 2 {
		flag.Usage()
		return
	}

	// Just make sure that the source is valid
	srcdb, _, err := spec.GetDataset(os.Args[1])
	if err != nil {
		fmt.Printf("invalid source dataset: %s\n", os.Args[1])
		fmt.Printf("%s\n", err)
		return
	}
	srcdb.Close()

	db, ds, err := spec.GetDataset(os.Args[2])
	if err != nil {
		fmt.Printf("invalid destination dataset: %s\n", os.Args[2])
		fmt.Printf("%s\n", err)
		return
	}
	defer db.Close()

	// Create our types.
	optionString := types.MakeUnionType(types.StringType, nothingType)
	optionNumber := types.MakeUnionType(types.NumberType, nothingType)
	optionBool := types.MakeUnionType(types.BoolType, nothingType)

	commentType = MakeStructType("Comment", []FieldType{
		{"id", types.NumberType},
		{"time", types.NumberType},

		{"text", optionString},
		{"by", optionString},

		{"deleted", optionBool},
		{"dead", optionBool},

		{"comments", types.MakeListType(types.MakeCycleType(0))},
	})

	storyType = MakeStructType("Story", []FieldType{
		{"id", types.NumberType},
		{"time", types.NumberType},

		{"title", optionString},
		{"url", optionString},
		{"text", optionString},
		{"by", optionString},

		{"deleted", optionBool},
		{"dead", optionBool},

		{"descendants", optionNumber},
		{"score", optionNumber},

		{"comments", types.MakeListType(commentType.t)},
	})

	_, ok := ds.MaybeHeadValue()
	if !ok {
		fmt.Println("doing the initial sync...")
		ds = littleSync(ds)
//		hv = ds.HeadValue()
	}

//	dstHead := hv.(types.Struct)
//	fmt.Println(dstHead.Hash())

}

func littleSync(ds datas.Dataset) datas.Dataset {
	srcdb, srcds, err := spec.GetDataset(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer srcdb.Close()

	head := srcds.HeadValue().(types.Struct)
	allItems := head.Get("items").(types.Map)

	newItem := make(chan types.Struct, 100)
	newStory := make(chan types.Value, 100)

	lastKey, _ := allItems.Last()
	lastIndex := int(lastKey.(types.Number))

	go func() {
		allItems.Iter(func(id, value types.Value) bool {
			//value := allItems.Get(index)
			item := value.(types.Struct)

			// Note that we're explicitly excluding items of type "job" and "poll" which may also be found in the list of top items.
			switch item.Type().Desc.(types.StructDesc).Name {
			case "story":
				myid := int(id.(types.Number))
				if myid == 8432709.0 {
					newItem <- item
				}
			}
			return false
		})
		close(newItem)
	}()

	workerPool(50, makeStories(allItems, newItem, newStory), func() {
		close(newStory)
	})

	streamData := make(chan types.Value, 100)
	newMap := types.NewStreamingMap(ds.Database(), streamData)

	start := time.Now()
	count := 0

	for story := range newStory {
		id := story.(types.Struct).Get("id")

		count++
		if count%1000 == 0 {
			n := int(id.(types.Number))
			dur := time.Since(start)
			eta := time.Duration(float64(dur) * float64(lastIndex-n) / float64(n))
			fmt.Printf("%d/%d %s\n", n, lastIndex, eta)
		}

		streamData <- id
		streamData <- story
	}
	close(streamData)

	fmt.Println("stream completed")

	stories := <-newMap

	fmt.Println("map created")
	fmt.Println("map length = ",stories.Len())

/*
	srcds, err = srcdb.CommitValue(srcds, types.NewStruct("HackerNoms", types.StructData{
		"stories": stories,
		"head":    types.String(srcds.Head().Hash().String()),
	}))
	if err != nil {
		panic(err)
	}
*/
	return ds
}

func makeStories(allItems types.Map, newItem <-chan types.Struct, newStory chan<- types.Value) func() {
	return func() {
		for item := range newItem {
			id := item.Get("id")
			fmt.Printf("working on story %d\n", int(id.(types.Number)))

			// Known stubs with just id and type
			if item.Type().Desc.(types.StructDesc).Len() == 2 {
				item.Get("type") // or panic
				continue
			}

			newStory <- NewStructWithType(storyType, types.ValueSlice{
				id,
				item.Get("time"),
				OptionGet(item, "title"),
				OptionGet(item, "url"),
				OptionGet(item, "text"),
				OptionGet(item, "by"),
				OptionGet(item, "deleted"),
				OptionGet(item, "dead"),
				OptionGet(item, "descendants"),
				OptionGet(item, "score"),
				comments(item, allItems),
			})
		}
	}
}

func OptionGet(st types.Struct, field string) types.Value {
	value, ok := st.MaybeGet(field)
	if ok {
		return value
	} else {
		return nothing
	}
}

func SomeOf(v types.Value) types.Value {
	if v.Type() == nothingType {
		panic("nothing!")
	}
	return v
}

func SomeOr(v types.Value, def types.Value) types.Value {
	if v.Type() == nothingType {
		return def
	}
	return v
}

// Process children; |item| may be a story or a comment.
func comments(item types.Value, allItems types.Map) types.Value {
	ret := types.NewList()

	c, ok := item.(types.Struct).MaybeGet("kids")
	if ok {
		c.(types.List).IterAll(func(id types.Value, _ uint64) {
			value, ok := allItems.MaybeGet(id)
			if !ok {
				fmt.Printf("unable to look up %d from %d\n", int(id.(types.Number)), int(item.(types.Struct).Get("id").(types.Number)))
				//panic(fmt.Sprintf("unable to look up %d from %d", int(id.(types.Number)), int(item.(types.Struct).Get("id").(types.Number))))
				return
			}

			subitem := value.(types.Struct)

			// Ignore stubs and zombies
			_, ok = subitem.MaybeGet("time")
			if !ok {
				return
			}

			comm := NewStructWithType(commentType, types.ValueSlice{
				id,
				subitem.Get("time"),
				OptionGet(subitem, "text"),
				OptionGet(subitem, "by"),
				OptionGet(subitem, "deleted"),
				OptionGet(subitem, "dead"),
				comments(subitem, allItems),
			})
			ret = ret.Append(comm)
		})
	}

	return ret
}

type StructType struct {
	t     *types.Type
	xform []int
}

type FieldType struct {
	name string
	t    *types.Type
}

type SortableFields struct {
	xform  []int
	fields []FieldType
}

func (s SortableFields) Len() int      { return len(s.xform) }
func (s SortableFields) Swap(i, j int) { s.xform[i], s.xform[j] = s.xform[j], s.xform[i] }
func (s SortableFields) Less(i, j int) bool {
	return s.fields[s.xform[i]].name < s.fields[s.xform[j]].name
}

func MakeStructType(name string, fields []FieldType) *StructType {
	xform := make([]int, len(fields))

	for idx, _ := range xform {
		xform[idx] = idx
	}

	sort.Sort(SortableFields{xform: xform, fields: fields})

	ns := make([]string, len(fields))
	ts := make([]*types.Type, len(fields))

	for to, from := range xform {
		ns[to] = fields[from].name
		ts[to] = fields[from].t
	}

	t := types.MakeStructType(name, ns, ts)

	return &StructType{t, xform}
}

func NewStructWithType(t *StructType, values types.ValueSlice) types.Value {
	v := make(types.ValueSlice, len(values))

	for to, from := range t.xform {
		v[to] = values[from]
	}

	return types.NewStructWithType(t.t, v)
}

func workerPool(count int, work func(), done func()) {
	workerDone := make(chan bool, 1)
	for i := 0; i < count; i += 1 {
		go func() {
			work()
			workerDone <- true
		}()
	}

	go func() {
		for i := 0; i < count; i += 1 {
			_ = <-workerDone
		}
		close(workerDone)
		done()
	}()
}

type RedisConfig struct {
	Hostname string
	Port     string
}

func (c *RedisConfig) Connect_string() string {
	connect := fmt.Sprint(c.Hostname, ":", c.Port)
	return connect
}

func NewRedisConfig() *RedisConfig {
	cfg := &RedisConfig{
		Hostname: "localhost",
		Port:     "6379",
	}
	return cfg
}

func getRedisConn() (c redis.Conn){

	cfg := NewRedisConfig()
	connect_string := cfg.Connect_string()
	c, err := redis.Dial("tcp", connect_string)
	if err != nil {
		panic(err)
	}
	return c
	// defer c.Close()
}
