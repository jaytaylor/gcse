// Package configs define and load all configurations. It depends on no othe GCSE packages.
package configs

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/golangplus/strings"

	"github.com/daviddengcn/gcse/utils"
	"github.com/daviddengcn/go-easybi"
	"github.com/daviddengcn/go-ljson-conf"
	"github.com/daviddengcn/go-villa"
	"github.com/daviddengcn/sophie"
	"github.com/daviddengcn/sophie/kv"
)

const (
	fnCrawlerDB = "crawler"

	fnToCrawl = "tocrawl"
	FnPackage = "package"
	FnPerson  = "person"
	// key: RawString, value: DocInfo
	FnDocs    = "docs"
	FnNewDocs = "newdocs"

	FnStore = "store"
)

var (
	ServerAddr = ":8080"
	ServerRoot = villa.Path("./cmd/gcse-service-web")

	LoadTemplatePass = ""
	AutoLoadTemplate = false

	DataRoot = villa.Path("./data/")

	// producer: server, consumer: crawler
	ImportPath villa.Path

	// producer: crawler, consumer: indexer
	DBOutPath villa.Path

	// configures of crawler
	CrawlByGodocApi           = true
	CrawlGithubUpdate         = true
	CrawlerDuePerRun          = 1 * time.Hour
	CrawlerGithubClientID     = os.Getenv("GITHUB_CLIENT_ID")
	CrawlerGithubClientSecret = os.Getenv("GITHUB_CLIENT_SECRET")
	CrawlerGithubPersonal     = os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN")

	BiWebPath = "/bi"

	NonCrawlHosts          = stringsp.Set{}
	NonStorePackageRegexps = []string{}

	StoreDAddr = ":8081"

	LogDir = "/tmp"
)

func init() {
	log.SetFlags(log.Flags() | log.Lshortfile)

	conf, err := ljconf.Load("conf.json")
	if err != nil {
		// we must make sure configuration exist
		log.Fatal(err)
	}
	ServerAddr = conf.String("web.addr", ServerAddr)
	ServerRoot = conf.Path("web.root", ServerRoot)
	LoadTemplatePass = conf.String("web.loadtemplatepass", LoadTemplatePass)
	AutoLoadTemplate = conf.Bool("web.autoloadtemplate", AutoLoadTemplate)

	DataRoot = conf.Path("back.dbroot", DataRoot)

	if err := Mkdirs(); err != nil {
		log.Fatal(err)
	}

	ImportPath = DataRoot.Join("imports")
	if err := ImportPath.MkdirAll(0755); err != nil {
		log.Fatal(err)
	}

	DBOutPath = DataRoot.Join("dbout")
	if err := DBOutPath.MkdirAll(0755); err != nil {
		log.Fatal(err)
	}

	CrawlByGodocApi = conf.Bool("crawler.godoc", CrawlByGodocApi)
	CrawlGithubUpdate = conf.Bool("crawler.github_update", CrawlGithubUpdate)
	CrawlerDuePerRun = conf.Duration("crawler.due_per_run", CrawlerDuePerRun)

	ncHosts := conf.StringList("crawler.noncrawl_hosts", nil)
	NonCrawlHosts.Add(ncHosts...)

	CrawlerGithubClientID = conf.String("crawler.github.clientid", CrawlerGithubClientID)
	CrawlerGithubClientSecret = conf.String("crawler.github.clientsecret", CrawlerGithubClientSecret)
	CrawlerGithubPersonal = conf.String("crawler.github.personal", CrawlerGithubPersonal)

	NonStorePackageRegexps = conf.StringList("docdb.nonstore_regexps", nil)

	bi.DataPath = conf.String("bi.data_path", "/tmp/gcse.bolt")
	BiWebPath = conf.String("bi.web_path", BiWebPath)

	StoreDAddr = conf.String("stored.addr", StoreDAddr)

	LogDir = conf.String("log.dir", LogDir)
}

func DataRootFsPath() sophie.FsPath {
	return sophie.LocalFsPath(DataRoot.S())
}

func CrawlerDBPath() villa.Path {
	return DataRoot.Join(fnCrawlerDB)
}

func CrawlerDBFsPath() sophie.FsPath {
	return DataRootFsPath().Join(fnCrawlerDB)
}

func DocsDBPath() string {
	return DataRoot.Join(FnDocs).S()
}

func DocsDBFsPath() sophie.FsPath {
	return DataRootFsPath().Join(FnDocs)
}

func ToCrawlPath() string {
	return DataRoot.Join(fnToCrawl).S()
}

func ToCrawlFsPath() sophie.FsPath {
	return DataRootFsPath().Join(fnToCrawl)
}

func IndexPath() villa.Path {
	return DataRoot.Join("index")
}

func StoreBoltPath() string {
	return DataRoot.Join("store.bolt").S()
}

func FileCacheBoltPath() string {
	return DataRoot.Join("filecache.bolt").S()
}

func SetTestingDataPath() error {
	DataRoot = villa.Path(os.TempDir()).Join("gcse_testing")

	if err := DataRoot.RemoveAll(); err != nil {
		return err
	}

	if err := DataRoot.MkdirAll(0755); err != nil {
		return err
	}

	log.Printf("New DataRoot (for testing): %v", DataRoot)
	return nil
}

// Returns the segments imported from web site.
func ImportSegments() utils.Segments {
	return utils.Segments(ImportPath)
}

func DBOutSegments() utils.Segments {
	return utils.Segments(DBOutPath)
}

func IndexSegments() utils.Segments {
	return utils.Segments(IndexPath())
}

// Mkdirs initializes the necessary directory structure.
func Mkdirs() error {
	paths := []villa.Path{
		CrawlerDBPath(),
		villa.Path(DocsDBPath()),
		DataRoot.Join(fnCrawlerDB),
		DataRoot.Join(fnCrawlerDB).Join(FnNewDocs),
		DataRoot.Join(FnDocs),
		DataRoot.Join(fnToCrawl),
		DataRoot.Join(fnToCrawl).Join(FnPackage),
		DataRoot.Join(fnToCrawl).Join(FnPerson),
		villa.Path(DocsDBFsPath().Path),
	}
	for i, p := range paths {
		if err := p.MkdirAll(0755); err != nil {
			log.Fatalf("[i=%v] Error creating %s: %s", i, p, err)
		}
	}

	fpDocs := DocsDBFsPath()
	dirInput := kv.DirInput(fpDocs)
	if err := os.MkdirAll(dirInput.Path, os.FileMode(int(0700))); err != nil {
		return fmt.Errorf("config: creating directory %v: %v", dirInput.Path, err)
	}
	return nil

}
