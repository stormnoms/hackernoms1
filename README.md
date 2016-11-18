## Hacker Noms

Scraper to pull hacker news data from firebase into the Noms database.

Sync to grab the raw data; organize to process the data into threads.

Deployable locally or as Docker containers.

### To get up and running

```
gop
cd src/github.com/stormasm/hackernoms/
vendetta -u
git submodule update --init --recursive
```

### To run sync

```
gr sync.go /tmp/my-noms::hn
```
