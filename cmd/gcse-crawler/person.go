package main

import (
	"context"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/daviddengcn/gddo/doc"
	"github.com/daviddengcn/go-easybi"
	"github.com/daviddengcn/sophie"
	"github.com/daviddengcn/sophie/kv"
	"github.com/daviddengcn/sophie/mr"
	"github.com/golangplus/errors"
	"github.com/golangplus/time"

	"github.com/daviddengcn/gcse"
	"github.com/daviddengcn/gcse/configs"
)

const (
	DefaultPersonAge = 100 * timep.Day
)

type PersonCrawler struct {
	crawlerMapper

	part       int
	failCount  int
	httpClient doc.HttpClient
}

func pushPerson(p *gcse.Person) {
	for _, pkg := range p.Packages {
		appendNewPackage(pkg, "user:"+p.Id)
	}
	cDB.SchedulePerson(p.Id, time.Now().Add(time.Duration(float64(DefaultPersonAge)*(1+(rand.Float64()-0.5)*0.2))))
}

// OnlyMapper.Map
func (pc *PersonCrawler) Map(key, val sophie.SophieWriter, c []sophie.Collector) error {
	ctx := context.Background()

	if time.Now().After(AppStopTime) {
		log.Printf("[Part %d] Timeout(key = %v), PersonCrawler returns EOM", pc.part, key)
		return mr.EOM
	}

	id := string(*key.(*sophie.RawString))
	// ent := val.(*gcse.CrawlingEntry)
	log.Printf("[Part %d] Crawling person %v\n", pc.part, id)

	p, err := gcse.CrawlPerson(ctx, pc.httpClient, id)
	if err != nil {
		bi.AddValue(bi.Sum, "crawler.person.failed", 1)
		pc.failCount++
		log.Printf("[Part %d] Crawling person %s failed: %v", pc.part, id, err)

		cDB.SchedulePerson(id, time.Now().Add(12*time.Hour))

		if pc.failCount >= 10 || strings.Contains(err.Error(), "403") {
			log.Printf("WARNING: \"403\" found in error message \"%s\"", err)

			durToSleep := 10 * time.Minute
			if time.Now().Add(durToSleep).After(AppStopTime) {
				log.Printf("[Part %d] Timeout(key = %v), PersonCrawler returns EOM", pc.part, key)
				return mr.EOM
			}

			log.Printf("[Part %d] Last ten crawling persons failed, sleep for a while...(current: %s)", pc.part, id)
			time.Sleep(durToSleep)
			pc.failCount = 0
		}
		return nil
	}
	bi.AddValue(bi.Sum, "crawler.person.success", 1)
	log.Printf("[Part %d] Crawled person %s success!", pc.part, id)
	pushPerson(p)
	log.Printf("[Part %d] Push person %s success", pc.part, id)
	pc.failCount = 0

	time.Sleep(10 * time.Second)

	return nil
}

type PeresonCrawlerFactory struct {
	httpClient doc.HttpClient
}

func (pcf PeresonCrawlerFactory) NewMapper(part int) mr.OnlyMapper {
	pc := &PersonCrawler{
		part:       part,
		httpClient: pcf.httpClient,
	}
	return pc
}

// crawl packages, send error back to end
func crawlPersons(httpClient doc.HttpClient, fpToCrawlPerson sophie.FsPath, end chan error) {
	timeout := configs.CrawlerDuePerRun + crawlOverdueLimit

	time.AfterFunc(timeout, func() {
		end <- errorsp.NewWithStacks("Crawling persons timed out after %s", timeout)
	})

	end <- func() error {
		job := mr.MapOnlyJob{
			Source: []mr.Input{
				kv.DirInput(fpToCrawlPerson),
			},
			NewMapperF: func(src, part int) mr.OnlyMapper {
				return &PersonCrawler{
					part:       part,
					httpClient: httpClient,
				}
			},
		}
		if err := job.Run(); err != nil {
			log.Printf("crawlPersons: job.Run failed: %v", err)
			return err
		}
		return nil
	}()
}
