package github

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	gpb "github.com/daviddengcn/gcse/shared/proto"
	"github.com/daviddengcn/gcse/spider"
	"github.com/daviddengcn/gddo/doc"
	"github.com/golang/protobuf/ptypes"
	"github.com/golangplus/bytes"
	"github.com/golangplus/errors"
	"github.com/golangplus/strings"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

var (
	ErrInvalidPackage    = errors.New("the package is not a Go package")
	ErrInvalidRepository = errors.New("the repository is not found")
	ErrRateLimited       = errors.New("Github rate limited")

	MinAllowedRateQuota       = 0.1
	RateLimitQuotaCheckPeriod = 30 * time.Second // 1 * time.Minute
)

type Spider struct {
	client *github.Client

	FileCache     spider.FileCache
	accessToken   string
	httpClient    doc.HttpClient
	lastRateCheck time.Time
	remaining     int
}

func NewSpiderWithToken(token string, httpClient doc.HttpClient) *Spider {
	hc := http.DefaultClient
	if token != "" {
		hc = oauth2.NewClient(oauth2.NoContext, oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		))
	}
	client := github.NewClient(hc)
	s := &Spider{
		client:      client,
		FileCache:   spider.NullFileCache{},
		accessToken: token,
		httpClient:  httpClient,
	}
	return s
}

type roundTripper map[string]string

func (rt roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	log.Printf("URI: %v", req.URL.RequestURI())
	body, ok := rt[req.URL.RequestURI()]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Request:    req,
			Body:       bytesp.NewPSlice([]byte("not found")),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       bytesp.NewPSlice([]byte(body)),
		Request:    req,
	}, nil
}

func NewSpiderWithContents(contents map[string]string) *Spider {
	hc := &http.Client{
		Transport: roundTripper(contents),
	}
	c := github.NewClient(hc)
	return &Spider{
		client:    c,
		FileCache: spider.NullFileCache{},
	}
}

type User struct {
	Repos map[string]*gpb.RepoInfo
}

func (s *Spider) waitForRate() error {
	// TODO: Implement proper GH API check for remaining calls.
	if s.accessToken == "" {
		time.Sleep(time.Second)
		return nil
	}

	if time.Now().Sub(s.lastRateCheck) < RateLimitQuotaCheckPeriod {
		return nil
	}

	rem, limit, err := s.checkRateQuota()
	if err != nil {
		return err
	}

	pct := float64(rem) / float64(limit)

	log.Printf("Github API quota rem/limit: %v/%v (%v%% available)", rem, limit, pct)

	if pct < MinAllowedRateQuota {
		log.Printf("Less than %v% github API quota is remaining, waiting to recover 2x this amount", MinAllowedRateQuota*100)

		for {
			time.Sleep(30 * time.Second)

			rem, limit, err := s.checkRateQuota()
			if err != nil {
				log.Printf("Error checking github rate-limit quota: %s", err)
				continue
			}

			newPct := float64(rem) / float64(limit)

			if newPct/2 > pct {
				log.Printf("Recovered 2x github rate-limit quota, resuming operation")
				break
			}
		}
	}

	s.lastRateCheck = time.Now()

	return nil
	//	r := s.client.Rate()
	//	if r.Limit == 0 {
	//		// no rate info yet
	//		return nil
	//	}
	//	if r.Remaining > 0 {
	//		return nil
	//	}
	//	d := r.Reset.Time.Sub(time.Now())
	//	if d > time.Minute {
	//		return errorsp.WithStacksAndMessage(ErrRateLimited, "time to wait: %v", d)
	//	}
	//	log.Printf("Quota used up (limit = %d), sleep until %v", r.Limit, r.Reset.Time)
	//	timep.SleepUntil(r.Reset.Time)
	//	return nil
}

func (s *Spider) checkRateQuota() (rem int, limit int, err error) {
	var (
		checkURL = fmt.Sprintf("https://api.github.com/rate_limit?access_token=%v", s.accessToken)
		req      *http.Request
		resp     *http.Response
	)

	if req, err = http.NewRequest("", checkURL, nil); err != nil {
		err = errorsp.WithStacksAndMessage(err, "creating http request to check github rate-limit quota")
		return
	}

	if resp, err = s.httpClient.Do(req); err != nil {
		err = errorsp.WithStacksAndMessage(err, "checking remaining github rate-limit quota")
		return
	}

	var (
		remStr   = resp.Header.Get("X-RateLimit-Remaining")
		limitStr = resp.Header.Get("X-RateLimit-Limit")
	)

	if rem, err = strconv.Atoi(remStr); err != nil {
		err = errorsp.WithStacksAndMessage(err, fmt.Sprintf("failed to convert github remaining header string %q into an int", limitStr))
		return
	}
	if limit, err = strconv.Atoi(limitStr); err != nil {
		err = errorsp.WithStacksAndMessage(err, fmt.Sprintf("failed to convert github limit header string %q into an int", limitStr))
		return
	}

	return
}

func repoInfoFromGithub(repo *github.Repository) *gpb.RepoInfo {
	ri := &gpb.RepoInfo{
		Description: stringsp.Get(repo.Description),
		Stars:       int32(getInt(repo.StargazersCount)),
	}
	ri.CrawlingTime, _ = ptypes.TimestampProto(time.Now())
	ri.LastUpdated, _ = ptypes.TimestampProto(getTimestamp(repo.PushedAt).Time)
	if repo.Source != nil {
		ri.Source = stringsp.Get(repo.Source.Name)
	}
	return ri
}

func (s *Spider) ReadUser(ctx context.Context, name string) (*User, error) {
	s.waitForRate()
	repos, _, err := s.client.Repositories.List(ctx, name, nil)
	if err != nil {
		return nil, errorsp.WithStacksAndMessage(err, "Repositories.List %v failed", name)
	}
	user := &User{}
	for _, repo := range repos {
		repoName := stringsp.Get(repo.Name)
		if repoName == "" {
			continue
		}
		if user.Repos == nil {
			user.Repos = make(map[string]*gpb.RepoInfo)
		}
		user.Repos[repoName] = repoInfoFromGithub(repo)
	}
	return user, nil
}

func (s *Spider) ReadRepository(ctx context.Context, user string, name string) (*gpb.RepoInfo, error) {
	s.waitForRate()
	repo, _, err := s.client.Repositories.Get(ctx, user, name)
	if err != nil {
		if isNotFound(err) {
			return nil, errorsp.WithStacksAndMessage(ErrInvalidRepository, "respository github.com/%v/%v not found", user, name)
		}
		return nil, errorsp.WithStacks(err)
	}
	return repoInfoFromGithub(repo), nil
}

func (s *Spider) getFile(ctx context.Context, user string, repo, path string) (string, error) {
	s.waitForRate()
	// TODO switch to DownloadContents
	c, _, _, err := s.client.Repositories.GetContents(ctx, user, repo, path, nil)
	if err != nil {
		return "", errorsp.WithStacks(err)
	}
	if c.GetType() != "file" {
		return "", errorsp.NewWithStacks("Contents of %v/%v/%v is not a file: %v", user, repo, path, stringsp.Get(c.Type))
	}
	body, err := c.GetContent()
	return body, errorsp.WithStacks(err)
}

func isReadmeFile(fn string) bool {
	fn = fn[:len(fn)-len(path.Ext(fn))]
	return strings.ToLower(fn) == "readme"
}

var buildTags stringsp.Set = stringsp.NewSet("linux", "386", "darwin", "cgo")

func buildIgnored(comments []*ast.CommentGroup) bool {
	for _, g := range comments {
		for _, c := range g.List {
			items, ok := stringsp.MatchPrefix(c.Text, "// +build ")
			if !ok {
				continue
			}
			for _, item := range strings.Split(items, " ") {
				for _, tag := range strings.Split(item, ",") {
					tag, _ = stringsp.MatchPrefix(tag, "!")
					if strings.HasPrefix(tag, "go") || buildTags.Contain(tag) {
						continue
					}
					return true
				}
			}
		}
	}
	return false
}

var (
	goFileInfo_ShouldIgnore = gpb.GoFileInfo{Status: gpb.GoFileInfo_ShouldIgnore}
	goFileInfo_ParseFailed  = gpb.GoFileInfo{Status: gpb.GoFileInfo_ParseFailed}
)

func parseGoFile(path string, body string, info *gpb.GoFileInfo) {
	info.IsTest = strings.HasSuffix(path, "_test.go")
	fs := token.NewFileSet()
	goF, err := parser.ParseFile(fs, "", body, parser.ImportsOnly|parser.ParseComments)
	if err != nil {
		log.Printf("Parsing file %v failed: %v", path, err)
		if info.IsTest {
			*info = goFileInfo_ShouldIgnore
		} else {
			*info = goFileInfo_ParseFailed
		}
		return
	}
	if buildIgnored(goF.Comments) {
		*info = goFileInfo_ShouldIgnore
		return
	}
	info.Status = gpb.GoFileInfo_ParseSuccess
	for _, imp := range goF.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		info.Imports = append(info.Imports, p)
	}
	info.Name = goF.Name.Name
	if goF.Doc != nil {
		info.Description = goF.Doc.Text()
	}
}

func calcFullPath(user string, repo string, path string, fn string) string {
	full := "github.com/" + user + "/" + repo
	if !strings.HasPrefix(path, "/") {
		full += "/"
	}
	full += path
	if !strings.HasSuffix(full, "/") {
		full += "/"
	}
	full += fn
	return full
}

func isTooLargeError(err error) bool {
	errResp, ok := errorsp.Cause(err).(*github.ErrorResponse)
	if !ok {
		return false
	}
	for _, e := range errResp.Errors {
		if e.Code == "too_large" {
			return true
		}
	}
	return false
}

func isNotFound(err error) bool {
	errResp, ok := errorsp.Cause(err).(*github.ErrorResponse)
	if !ok {
		return false
	}
	return errResp.Response.StatusCode == http.StatusNotFound
}

func folderInfoFromGithub(rc *github.RepositoryContent) *gpb.FolderInfo {
	return &gpb.FolderInfo{
		Name:    getString(rc.Name),
		Path:    getString(rc.Path),
		Sha:     getString(rc.SHA),
		HtmlUrl: getString(rc.HTMLURL),
	}
}

type Package struct {
	Name        string // package "name"
	Path        string // Relative path to the repository
	Description string
	ReadmeFn    string // No directory info
	ReadmeData  string // Raw content, cound be md, txt, etc.
	Imports     []string
	TestImports []string
}

// Even an error is returned, the folders may still contain useful elements.
func (s *Spider) ReadPackage(ctx context.Context, user string, repo string, path string) (*Package, []*gpb.FolderInfo, error) {
	s.waitForRate()
	_, cs, _, err := s.client.Repositories.GetContents(ctx, user, repo, path, nil)
	if err != nil {
		if isNotFound(err) {
			return nil, nil, errorsp.WithStacksAndMessage(ErrInvalidPackage, "GetContents %v %v %v returns 404", user, repo, path)
		}
		errResp, _ := errorsp.Cause(err).(*github.ErrorResponse)
		return nil, nil, errorsp.WithStacksAndMessage(err, "GetContents %v %v %v failed: %v", user, repo, path, errResp)
	}
	var folders []*gpb.FolderInfo
	for _, c := range cs {
		if getString(c.Type) != "dir" {
			continue
		}
		folders = append(folders, folderInfoFromGithub(c))
	}
	pkg := Package{
		Path: path,
	}
	var imports stringsp.Set
	var testImports stringsp.Set
	// Process files
	for _, c := range cs {
		fn := getString(c.Name)
		if getString(c.Type) != "file" {
			continue
		}
		sha := getString(c.SHA)
		cPath := path + "/" + fn
		switch {
		case strings.HasSuffix(fn, ".go"):
			fi, err := func() (*gpb.GoFileInfo, error) {
				fi := &gpb.GoFileInfo{}
				if s.FileCache.Get(sha, fi) {
					log.Printf("Cache for %v found(sha:%q)", calcFullPath(user, repo, path, fn), sha)
					return fi, nil
				}
				body, err := s.getFile(ctx, user, repo, cPath)
				if err != nil {
					if isTooLargeError(err) {
						*fi = goFileInfo_ShouldIgnore
					} else {
						// Temporary error
						return nil, err
					}
				} else {
					parseGoFile(cPath, body, fi)
				}
				s.FileCache.Set(sha, fi)
				log.Printf("Save file cache for %v (sha:%q)", calcFullPath(user, repo, path, fn), sha)
				return fi, nil
			}()
			if err != nil {
				return nil, folders, err
			}
			if fi.Status == gpb.GoFileInfo_ParseFailed {
				return nil, folders, errorsp.WithStacksAndMessage(ErrInvalidPackage, "fi.Status is ParseFailed")
			}
			if fi.Status == gpb.GoFileInfo_ShouldIgnore {
				continue
			}
			if fi.IsTest {
				testImports.Add(fi.Imports...)
			} else {
				if pkg.Name != "" {
					if fi.Name != pkg.Name {
						return nil, folders, errorsp.WithStacksAndMessage(ErrInvalidPackage,
							"conflicting package name processing file %v: %v vs %v", cPath, fi.Name, pkg.Name)
					}
				} else {
					pkg.Name = fi.Name
				}
				if fi.Description != "" {
					if pkg.Description != "" && !strings.HasSuffix(pkg.Description, "\n") {
						pkg.Description += "\n"
					}
					pkg.Description += fi.Description
				}
				imports.Add(fi.Imports...)
			}
		case isReadmeFile(fn):
			body, err := s.getFile(ctx, user, repo, cPath)
			if err != nil {
				log.Printf("Get file %v failed: %v", cPath, err)
				continue
			}
			pkg.ReadmeFn = fn
			pkg.ReadmeData = string(body)
		}
	}
	if pkg.Name == "" {
		return nil, folders, errorsp.WithStacksAndMessage(ErrInvalidPackage, "package name is not set")
	}
	pkg.Imports = imports.Elements()
	pkg.TestImports = testImports.Elements()
	return &pkg, folders, nil
}

func (s *Spider) SearchRepositories(ctx context.Context, q string) ([]github.Repository, error) {
	if !strings.Contains(q, "language:go") {
		q += " language:go"
		q = strings.TrimSpace(q)
	}
	s.waitForRate()
	res, _, err := s.client.Search.Repositories(ctx, q, &github.SearchOptions{})
	if err != nil {
		return nil, errorsp.WithStacksAndMessage(err, "Search.Repositories %q failed: %+v", q, err)
	}
	return res.Repositories, nil
}

func (s *Spider) RepoBranchSHA(ctx context.Context, owner string, repo string, branch string) (sha string, err error) {
	if err := s.waitForRate(); err != nil {
		return "", err
	}
	b, _, err := s.client.Repositories.GetBranch(ctx, owner, repo, branch)
	if err != nil {
		if isNotFound(err) {
			return "", errorsp.WithStacksAndMessage(ErrInvalidRepository, "GetBranch %v %v %v failed", owner, repo, branch)
		}
		return "", errorsp.WithStacksAndMessage(err, "GetBranch %v %v %v failed", owner, repo, branch)
	}
	if b.Commit == nil {
		return "", nil
	}
	return stringsp.Get(b.Commit.SHA), nil
}

func (s *Spider) getTree(ctx context.Context, owner string, repo string, sha string, recursive bool) (*github.Tree, error) {
	if err := s.waitForRate(); err != nil {
		return nil, err
	}
	tree, _, err := s.client.Git.GetTree(ctx, owner, repo, sha, true)
	if err != nil {
		if isNotFound(err) {
			return nil, errorsp.WithStacksAndMessage(ErrInvalidRepository, "GetTree %v %v %v failed", owner, repo, sha)
		}
		return nil, errorsp.WithStacksAndMessage(err, "GetTree %v %v %v failed", owner, repo, sha)
	}
	return tree, nil
}

// ReadRepo reads all packages of a repository.
// For pkg given to f, it will not be reused.
// path in f is relative to the repository path.
func (s *Spider) ReadRepo(ctx context.Context, user string, repo string, sha string, f func(path string, pkg *gpb.Package) error) error {
	tree, err := s.getTree(ctx, user, repo, sha, true)
	if err != nil {
		return err
	}
	pkgs := make(map[string][]github.TreeEntry)
	for _, te := range tree.Entries {
		if stringsp.Get(te.Type) != "blob" {
			continue
		}
		p := stringsp.Get(te.Path)
		if p == "" {
			continue
		}
		d := path.Dir(p)
		if d == "." {
			d = ""
		} else {
			d = "/" + d
		}
		pkgs[d] = append(pkgs[d], te)
	}
	log.Printf("pkgs: %v", pkgs)
	for d, teList := range pkgs {
		pkg := gpb.Package{
			Path: d,
		}
		var imports stringsp.Set
		var testImports stringsp.Set
		for _, te := range teList {
			fn := path.Base(*te.Path)
			cPath := *te.Path
			sha := *te.SHA
			switch {
			case strings.HasSuffix(fn, ".go"):
				fi, err := func() (*gpb.GoFileInfo, error) {
					fi := &gpb.GoFileInfo{}
					if s.FileCache.Get(sha, fi) {
						log.Printf("Cache for %v found(sha:%q)", "github.com/"+user+"/"+cPath, sha)
						return fi, nil
					}
					body, err := s.getFile(ctx, user, repo, cPath)
					if err != nil {
						if isTooLargeError(err) {
							*fi = goFileInfo_ShouldIgnore
						} else {
							// Temporary error
							return nil, err
						}
					} else {
						parseGoFile(cPath, body, fi)
					}
					s.FileCache.Set(sha, fi)
					log.Printf("Save file cache for %v (sha:%q)", "github.com/"+user+"/"+cPath, sha)
					return fi, nil
				}()
				if err != nil {
					return err
				}
				if fi.Status == gpb.GoFileInfo_ParseFailed {
					return errorsp.WithStacksAndMessage(ErrInvalidPackage, "fi.Status is ParseFailed")
				}
				if fi.Status == gpb.GoFileInfo_ShouldIgnore {
					continue
				}
				if fi.IsTest {
					testImports.Add(fi.Imports...)
				} else {
					if pkg.Name != "" {
						if fi.Name != pkg.Name {
							return errorsp.WithStacksAndMessage(ErrInvalidPackage, "conflicting package name processing file %v: %v vs %v", cPath, fi.Name, pkg.Name)
						}
					} else {
						pkg.Name = fi.Name
					}
					if fi.Description != "" {
						if pkg.Description != "" && !strings.HasSuffix(pkg.Description, "\n") {
							pkg.Description += "\n"
						}
						pkg.Description += fi.Description
					}
					imports.Add(fi.Imports...)
				}
			case isReadmeFile(fn):
				body, err := s.getFile(ctx, user, repo, cPath)
				if err != nil {
					log.Printf("Get file %v failed: %v", cPath, err)
					continue
				}
				pkg.ReadmeFn = fn
				pkg.ReadmeData = string(body)
			}
		}
		if pkg.Name == "" {
			continue
		}
		pkg.Imports = imports.Elements()
		pkg.TestImports = testImports.Elements()
		if err := errorsp.WithStacks(f(d, &pkg)); err != nil {
			return err
		}
	}
	return nil
}
