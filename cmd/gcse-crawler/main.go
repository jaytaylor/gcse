/*
	GCSE Crawler background program.
*/
package main

import (
	"context"
	"flag"
	"io"
	"log"
	"runtime"
	"time"

	"github.com/golangplus/errors"
	"github.com/golangplus/fmt"

	"github.com/daviddengcn/bolthelper"
	"github.com/daviddengcn/gcse"
	"github.com/daviddengcn/gcse/configs"
	"github.com/daviddengcn/gcse/spider"
	"github.com/daviddengcn/gcse/spider/github"
	"github.com/daviddengcn/gcse/utils"
	"github.com/daviddengcn/gddo/doc"
	"github.com/daviddengcn/go-easybi"
	"github.com/daviddengcn/go-villa"
	"github.com/daviddengcn/sophie"
	"github.com/daviddengcn/sophie/kv"
)

var (
	AppStopTime time.Time
	cDB         *gcse.CrawlerDB
)

func init() {
	if configs.CrawlerGithubClientID != "" {
		log.Printf("Github clientid: %s", configs.CrawlerGithubClientID)
		log.Printf("Github clientsecret: %s", configs.CrawlerGithubClientSecret)
		doc.SetGithubCredentials(configs.CrawlerGithubClientID, configs.CrawlerGithubClientSecret)
	}
	doc.SetUserAgent("Go-Search(http://go-search.org/)")
}

func syncDatabases() {
	utils.DumpMemStats()
	log.Printf("Synchronizing databases to disk...")
	if err := cDB.Sync(); err != nil {
		log.Fatalf("cdb.Sync() failed: %v", err)
	}
	utils.DumpMemStats()
	runtime.GC()
	utils.DumpMemStats()
}

func loadAllDocsPkgs(in kv.DirInput) error {
	cnt, err := in.PartCount()
	if err != nil {
		return err
	}
	for part := 0; part < cnt; part++ {
		c, err := in.Iterator(part)
		if err != nil {
			return err
		}
		for {
			var key sophie.RawString
			var val gcse.DocInfo
			if err := c.Next(&key, &val); err != nil {
				if errorsp.Cause(err) == io.EOF {
					break
				}
				return err
			}
			allDocsPkgs.Add(string(key))
			// value is ignored
		}
	}
	return nil
}

type crawlerMapper struct {
}

// Mapper interface
func (crawlerMapper) NewKey() sophie.Sophier {
	return new(sophie.RawString)
}

// Mapper interface
func (crawlerMapper) NewVal() sophie.Sophier {
	return new(gcse.CrawlingEntry)
}

// Mapper interface
func (crawlerMapper) MapEnd(c []sophie.Collector) error {
	return nil
}

func cleanTempDir() {
	tmpFn := villa.Path("/tmp/gddo")
	if err := tmpFn.RemoveAll(); err != nil {
		log.Printf("Delete %v failed: %v", tmpFn, err)
	}
}

func main() {
	ctx := context.Background()

	log.Printf("Using personal github token: %v", configs.CrawlerGithubPersonal)
	gcse.GithubSpider = github.NewSpiderWithToken(configs.CrawlerGithubPersonal)

	fileCachePath := configs.FileCacheBoltPath()
	if db, err := bh.Open(fileCachePath, 0644, nil); err == nil {
		log.Printf("Using file cache %q", fileCachePath)
		gcse.GithubSpider.FileCache = spider.BoltFileCache{
			DB:         db,
			IncCounter: bi.Inc,
		}
	} else {
		log.Fatalf("Error: failed to open bolt file cache %q: %v", fileCachePath, err)
	}

	cleanTempDir()
	defer cleanTempDir()

	singlePackage := flag.String("pkg", "", "Crawling a single package")
	singleETag := flag.String("etag", "", "ETag for the single package crawling")
	singlePerson := flag.String("person", "", "Crawling a single person")

	flag.Parse()

	httpClient := gcse.GenHttpClient("")

	if *singlePerson != "" {
		log.Printf("Crawling single person %q ...", *singlePerson)
		p, err := gcse.CrawlPerson(ctx, httpClient, *singlePerson)
		if err != nil {
			fmtp.Printfln("Crawling person %q failed: %v", *singlePerson, err)
		} else {
			fmtp.Printfln("Person %s: %+v", *singlePerson, p)
		}
		log.Println("Crawler finished single person OK")
		return
	}

	if *singlePackage != "" {
		log.Printf("Crawling single package %q ...", *singlePackage)
		p, flds, err := gcse.CrawlPackage(ctx, httpClient, *singlePackage, *singleETag)
		if err != nil {
			fmtp.Printfln("Crawling package %q failed: %v\nfolders: %v", *singlePackage, err, flds)
		} else {
			fmtp.Printfln("Package %s: %+v\nfolders: %v", *singlePackage, p, flds)
		}
		log.Println("Crawler finished single package OK")
		return
	}

	log.Println("Crawler started...")

	if err := configs.Mkdirs(); err != nil {
		log.Fatalf("main: %s", err)
	}

	// Load CrawlerDB
	cDB = gcse.LoadCrawlerDB()

	fpDocs := configs.DocsDBFsPath()
	dirInput := kv.DirInput(fpDocs)
	if err := loadAllDocsPkgs(dirInput); err != nil {
		log.Fatalf("loadAllDocsPkgs: loading data from %v: %v", dirInput.Path, err)
	}
	log.Printf("%d docs loaded!", len(allDocsPkgs))

	AppStopTime = time.Now().Add(configs.CrawlerDuePerRun)

	//pathToCrawl := gcse.DataRoot.Join(gcse.FnToCrawl)
	fpCrawler := configs.CrawlerDBFsPath()
	fpToCrawl := configs.ToCrawlFsPath()

	fpNewDocs := fpCrawler.Join(configs.FnNewDocs)
	fpNewDocs.Remove()

	if err := processImports(); err != nil {
		log.Fatalf("processImports failed: %v", err)
	}

	pkgEnd := make(chan error, 1)
	go crawlPackages(httpClient, fpToCrawl.Join(configs.FnPackage), fpNewDocs, pkgEnd)

	psnEnd := make(chan error, 1)
	go crawlPersons(httpClient, fpToCrawl.Join(configs.FnPerson), psnEnd)

	errPkg, errPsn := <-pkgEnd, <-psnEnd
	bi.Flush()
	bi.Process()
	syncDatabases()
	if errPkg != nil || errPsn != nil {
		log.Fatalf("Some job may have failed, package: %v, person: %v", errPkg, errPsn)
	}

	log.Println("Crawler finished OK")
}
