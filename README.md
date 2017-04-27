## Hacker Noms

Scraper to pull hacker news data from firebase into the Noms database.

Sync to grab the raw data; organize to process the data into threads.

Deployable locally or as Docker containers.

### To get up and running

In this repo I removed the dependency on

https://github.com/dpw/vendetta

By pulling these repos and then checking them into vendor

https://github.com/attic-labs/noms/tree/40b28f94e528df4721772bf99e63d2d7ed7471c2

https://github.com/zabawaba99/firego/tree/3b73602864a1c8786cb73f5c8b20718573a97ebd

### To run sync

```
cd sync
go build
./sync ./mynoms::hn

OR

cd sync
go run sync.go /tmp/my-noms::hn
```

### To run organize

```
go run organize.go /tmp/my-noms::hn /tmp/my-noms-org::hn
```
