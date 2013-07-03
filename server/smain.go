/*
	GCSE HTTP server.
*/
package main

import (
	"github.com/daviddengcn/gcse"
	//	"fmt"
	"github.com/daviddengcn/go-code-crawl"
	"github.com/daviddengcn/go-index"
	"github.com/daviddengcn/go-villa"
	godoc "go/doc"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var templates = template.Must(template.ParseGlob(gcse.ServerRoot.Join(`web/*`).S()))

func init() {
	http.Handle("/css/", http.StripPrefix("/css/",
		http.FileServer(http.Dir(gcse.ServerRoot.Join("css").S()))))
	http.Handle("/images/", http.StripPrefix("/images/",
		http.FileServer(http.Dir(gcse.ServerRoot.Join("images").S()))))

	http.HandleFunc("/add", pageAdd)
	http.HandleFunc("/search", pageSearch)
	http.HandleFunc("/view", pageView)
	http.HandleFunc("/tops", pageTops)

	//	http.HandleFunc("/update", pageUpdate)

	http.HandleFunc("/", pageRoot)
}

type LogHandler struct{}

func (hdl LogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[B] %s %v %s", r.Method, r.RequestURI, r.RemoteAddr)
	http.DefaultServeMux.ServeHTTP(w, r)
	log.Printf("[E] %s %v %s", r.Method, r.RequestURI, r.RemoteAddr)
}

func main() {
	if err := gcse.ImportSegments.ClearUndones(); err != nil {
		log.Printf("CleanImportSegments failed: %v", err)
	}

	if err := loadIndex(); err != nil {
		log.Fatal(err)
	}
	go loadIndexLoop()

	log.Printf("ListenAndServe at %s ...", gcse.ServerAddr)

	http.ListenAndServe(gcse.ServerAddr, LogHandler{})
}

func pageRoot(w http.ResponseWriter, r *http.Request) {
	docCount := 0
	if indexDB != nil {
		docCount = indexDB.DocCount()
	}
	err := templates.ExecuteTemplate(w, "index.html", docCount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func pageAdd(w http.ResponseWriter, r *http.Request) {
	pkgsStr := r.FormValue("pkg")
	if pkgsStr != "" {
		pkgs := strings.Split(pkgsStr, "\n")
		log.Printf("%d packaged submitted!", len(pkgs))
		gcse.AppendPackages(pkgs)
	}

	err := templates.ExecuteTemplate(w, "add.html", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type SubProjectInfo struct {
	MarkedName template.HTML
	Package    string
	SubPath    string
	Info       string
}

type ShowDocInfo struct {
	*Hit
	Index         int
	Summary       template.HTML
	MarkedName    template.HTML
	MarkedPackage template.HTML
	Subs          []SubProjectInfo
}

type ShowResults struct {
	TotalResults int
	TotalEntries int
	Folded       int
	Docs         []ShowDocInfo
}

func markWord(word []byte) []byte {
	buf := villa.ByteSlice("<b>")
	template.HTMLEscape(&buf, word)
	buf.Write([]byte("</b>"))
	return buf
}

func markText(text string, tokens villa.StrSet,
	markFunc func([]byte) []byte) template.HTML {
	if len(text) == 0 {
		return ""
	}

	var outBuf villa.ByteSlice

	index.MarkText([]byte(text), gcse.CheckRuneType, func(token []byte) bool {
		// needMark
		return tokens.In(gcse.NormWord(string(token)))
	}, func(text []byte) error {
		// output
		template.HTMLEscape(&outBuf, text)
		return nil
	}, func(token []byte) error {
		outBuf.Write(markFunc(token))
		return nil
	})

	return template.HTML(string(outBuf))
}

type Range struct {
	start, count int
}

func (r Range) In(idx int) bool {
	return idx >= r.start && idx < r.start+r.count
}

func packageShowName(name, pkg string) string {
	if name != "" && name != "main" {
		return name
	}

	prj := gcc.ProjectOfPackage(pkg)

	if prj == "main" {
		return "main - " + prj
	}

	return "(" + prj + ")"
}

func showSearchResults(results *SearchResult, tokens villa.StrSet,
	r Range) *ShowResults {
	docs := make([]ShowDocInfo, 0, len(results.Hits))

	projToIdx := make(map[string]int)
	folded := 0

	cnt := 0
mainLoop:
	for _, d := range results.Hits {
		d.Name = packageShowName(d.Name, d.Package)

		parts := strings.Split(d.Package, "/")
		if len(parts) > 2 {
			for i := len(parts) - 1; i >= 2; i-- {
				pkg := strings.Join(parts[:i], "/")
				if idx, ok := projToIdx[pkg]; ok {
					markedName := markText(d.Name, tokens, markWord)
					if r.In(idx) {
						docsIdx := idx - r.start
						docs[docsIdx].Subs = append(docs[docsIdx].Subs,
							SubProjectInfo{
								MarkedName: markedName,
								Package:    d.Package,
								SubPath:    "/" + strings.Join(parts[i:], "/"),
								Info:       d.Synopsis,
							})
					}
					folded++
					continue mainLoop
				}
			}
		}

		//		if len(docs) >= 1000 {
		//			continue
		//		}

		projToIdx[d.Package] = cnt
		if r.In(cnt) {
			markedName := markText(d.Name, tokens, markWord)
			raw := selectSnippets(d.Description+"\n"+d.ReadmeData, tokens, 300)

			if d.StarCount < 0 {
				d.StarCount = 0
			}
			docs = append(docs, ShowDocInfo{
				Hit:           d,
				Index:         cnt + 1,
				MarkedName:    markedName,
				Summary:       markText(raw, tokens, markWord),
				MarkedPackage: markText(d.Package, tokens, markWord),
			})
		}
		cnt++
	}

	return &ShowResults{
		TotalResults: results.TotalResults,
		TotalEntries: cnt,
		Folded:       folded,
		Docs:         docs,
	}
}

const itemsPerPage = 10

func pageSearch(w http.ResponseWriter, r *http.Request) {
	// current page, 1-based
	p, err := strconv.Atoi(r.FormValue("p"))
	if err != nil {
		p = 1
	}

	startTime := time.Now()

	q := strings.TrimSpace(r.FormValue("q"))
	results, tokens, err := search(q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	showResults := showSearchResults(results, tokens,
		Range{(p - 1) * itemsPerPage, itemsPerPage})
	totalPages := (showResults.TotalEntries + itemsPerPage - 1) / itemsPerPage
	log.Printf("totalPages: %d", totalPages)
	var beforePages, afterPages []int
	for i := 1; i <= totalPages; i++ {
		if i < p && p-i < 10 {
			beforePages = append(beforePages, i)
		} else if i > p && i-p < 10 {
			afterPages = append(afterPages, i)
		}
	}

	prevPage, nextPage := p-1, p+1
	if prevPage < 0 || prevPage > totalPages {
		prevPage = 0
	}
	if nextPage < 0 || nextPage > totalPages {
		nextPage = 0
	}

	data := struct {
		Q           string
		Results     *ShowResults
		SearchTime  time.Duration
		BeforePages []int
		PrevPage    int
		CurrentPage int
		NextPage    int
		AfterPages  []int
		BottomQ     bool
	}{
		Q:           q,
		Results:     showResults,
		SearchTime:  time.Now().Sub(startTime),
		BeforePages: beforePages,
		PrevPage:    prevPage,
		CurrentPage: p,
		NextPage:    nextPage,
		AfterPages:  afterPages,
		BottomQ:     len(results.Hits) >= 5,
	}
	log.Printf("Search results ready")
	err = templates.ExecuteTemplate(w, "search.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	log.Printf("Search results rendered")
}

func pageView(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.FormValue("id"))
	if id != "" {
		var doc gcse.HitInfo

		if indexDB != nil {
			indexDB.Search(index.SingleFieldQuery("pkg", id),
				func(docID int32, data interface{}) error {
					doc, _ = data.(gcse.HitInfo)
					return nil
				})
		}
		/*
			//err, exists := ddb.Get(id, &doc)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			if !exists {
				fmt.Fprintf(w, `<html><body>No such entry!`)

				ent, _ := findCrawlingEntry(c, kindCrawlerPackage, id)
				if ent != nil {
					fmt.Fprintf(w, ` Scheduled to be crawled at %s`,
						ent.ScheduleTime.Format("2006-01-02 15:04:05"))
				} else {
					fmt.Fprintf(w, ` Not found yet!`)
				}
				fmt.Fprintf(w, ` Click to <a href="crawl?id=%s">crawl</a>.</body></html>`,
					template.URLQueryEscaper(id))
				return
			}
		*/

		if doc.StarCount < 0 {
			doc.StarCount = 0
		}

		var descHTML villa.ByteSlice
		godoc.ToHTML(&descHTML, doc.Description, nil)

		showReadme := len(doc.Description) < 10 && len(doc.ReadmeData) > 0

		if err := templates.ExecuteTemplate(w, "view.html", struct {
			gcse.HitInfo
			DescHTML   template.HTML
			ShowReadme bool
		}{
			HitInfo:    doc,
			DescHTML:   template.HTML(descHTML),
			ShowReadme: showReadme,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func pageTops(w http.ResponseWriter, r *http.Request) {
	err := templates.ExecuteTemplate(w, "tops.html", statTops())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
