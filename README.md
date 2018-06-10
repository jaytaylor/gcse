Go Search [![GoSearch](http://go-search.org/badge?id=github.com%2Fdaviddengcn%2Fgcse)](http://go-search.org/view?id=github.com%2Fdaviddengcn%2Fgcse)
=========

A keyword search engine helping people to find popular and relevant Go packages.

Online service: [Go Search](http://go-search.org/)

This is the root package with shared functions.

Sub-packages are commands for running:

* [HTTP Server](cmd/gcse-service-web): Searching and web service
* [ToCrawl](cmd/gcse-tocrawl): Find packages to crawl.
* [Crawler](cmd/gcse-crawler): Crawling package files.
* [MergeDocs](cmd/gcse-mergedocs): Merge crawled package files with doc DB.
* [Indexer](cmd/gcse-indexer): Analyzing package information and generating indexed data for searching.

Development
-----------

You'll need to perform the following steps to get the server up and running:

  0. Install gcse: `go get -u github.com/daviddengcn/gcse/...`
  1. Create a basic `conf.json` file, limiting the crawler to a one minute run: `{ "crawler": { "due_per_run": "1m" } }`
  2. Run the package finder: `gcse-tocrawl`
  3. Run the crawler: `gcse-crawler`
  4. Merge the crawled docs: `gcse-mergedocs`
  5. Run the indexer: `gcse-indexer`
  6. Run the server: `gcse-service-web`
  7. Visit [http://localhost:8080](http://localhost:8080) in your browser

LICENSE
-------
BSD license.
