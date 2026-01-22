package router

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"log/slog"

	"log"
	"mime"

	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	gzipped "github.com/lpar/gzipped/v2"
	"github.com/rickb777/servefiles"

	_ "github.com/browserutils/kooky/browser/all" // register cookie store finders!
	"github.com/joho/godotenv"
	t1 "github.com/senforsce/tndr"
)

type Plug func(Handler) Handler

type Handler func(c *Context)

type ErrorHandler func(error, *Context) error

type Context gin.Context

func newContext(w gin.ResponseWriter, r *http.Request, params gin.Params) *Context {
	return &Context{
		Writer:  w,
		Request: r,
		Params:  params,
		Keys:    make(map[any]any),
	}
}

func (c *Context) Param(name string) string {
	return c.Params.ByName(name)
}

func (c *Context) Cookie(name string) (string, error) {
	for _, cookie := range c.Request.Cookies() {
		if cookie != nil {
			if cookie.Name == name {
				return cookie.Value, nil
			}
		}

	}
	return "", fmt.Errorf("Cookie not found")
}

func (c *Context) SetCookie(name string, value string, maxAge int, path, location string, secure, httpOnly bool) error {
	// uses registered finders to find cookie store files in default locations
	// applies the passed filters "Valid", "DomainHasSuffix()" and "Name()" in order to the cookies

	expires := time.Now().AddDate(1, 0, 0)

	ck := http.Cookie{
		Name:    name,
		Domain:  location,
		Path:    "/",
		Expires: expires,
	}

	// value of cookie
	ck.Value = value

	// write the cookie to response
	http.SetCookie(c.Writer, &ck)

	return fmt.Errorf("Cookie not found")
}

func (c *Context) Query(name string) string {
	return c.Request.URL.Query().Get(name)
}

func (c *Context) FormValue(name string) string {
	return c.Request.FormValue(name)
}

func (c *Context) Render(component t1.Component) error {
	return component.Render(*c.Ctx(), c.Writer)
}

func (c *Context) Redirect(url string, code int) error {
	if code < http.StatusMultipleChoices || code > http.StatusTemporaryRedirect {
		return errors.New("invalid redirect code")
	}

	http.Redirect(c.Writer, c.Request, url, code)
	return nil
}

func (c *Context) JSON(status int, v any) error {
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(status)
	return json.NewDecoder(c.Request.Body).Decode(&v)
}

func (c *Context) Text(status int, t string) error {
	c.Writer.Header().Set("Content-Type", "text/plain")
	c.Writer.WriteHeader(status)
	_, err := c.Writer.Write([]byte(t))
	return err
}

func (c *Context) Set(key string, value any) {
	c.Keys[key] = value
}

func (c *Context) Get(key string) any {
	res := c.Keys[key]

	if res != nil {
		return res
	}

	return key
}

type Tndr0cean struct {
	ErrorHandler ErrorHandler
	router       *gin.Engine
	Logger       *slog.Logger
	plugs        []Plug
	Global       GlobalStorage
}

func New(logger *slog.Logger) *Tndr0cean {
	return &Tndr0cean{
		Logger:       logger,
		router:       gin.Default(),
		ErrorHandler: defaultErrorHandler,
		Global: GlobalStorage{
			Data:     make(map[string]string),
			Triples:  make([]any, 0),
			Prefixes: make(map[string]string),
		},
	}
}

// initialized by the first start
type GlobalStorage struct {
	Data           map[string]string
	Triples        any
	TriplesPointer any
	Prefixes       any
	Repo           any
	RepoName       string
}

func (t0 *Tndr0cean) S(key string, value string) {
	t0.Logger.Debug(fmt.Sprintf("TndrOcean: setting [%s]=!%s!", key, value))
	t0.Global.Data[key] = value
}

func (t0 *Tndr0cean) SetRepo(name string, repo any) {
	t0.Logger.Debug(fmt.Sprintln("TndrOcean: Repo "))
	t0.Global.Repo = repo
	t0.Global.RepoName = name
}

func (t0 *Tndr0cean) SetTriples(triples any, pointer any, prefixes any) {
	t0.Logger.Debug(fmt.Sprintln("TndrOcean: triples "))
	t0.Global.Triples = triples
	t0.Global.Prefixes = prefixes
	t0.Global.TriplesPointer = pointer
}

func (t0 *Tndr0cean) G(key string) string {
	return t0.Global.Data[key]
}

type methodNotAllowedHandler struct {
	handler Handler
}

type notFoundHandler struct {
	handler Handler
}

func (h notFoundHandler) ServeHTTP(w gin.ResponseWriter, r *http.Request) {
	ctx := newContext(w, r, gin.Params{})
	h.handler(ctx)
}

func (h methodNotAllowedHandler) ServeHTTP(w gin.ResponseWriter, r *http.Request) {
	ctx := newContext(w, r, gin.Params{})
	h.handler(ctx)
}

func (s *Tndr0cean) MethodNotAllowed(h gin.HandlerFunc) {
	s.router.NoMethod(h)
}

func (s *Tndr0cean) NotFound(h gin.HandlerFunc) {
	s.router.NoRoute(h)
}

func (s *Tndr0cean) Plug(plugs ...Plug) {
	s.plugs = append(s.plugs, plugs...)
}

func WithMid(h Handler, h2 Handler) Handler {
	return func(c *Context) {
		h(c)

		h2(c)
	}
}

type Options struct {
	EmbeddedDir embed.FS
	Embedded    bool
	StaticRoot  []string
}

func (s *Tndr0cean) Start(o Options) error {
	if err := godotenv.Load(); err != nil {
		return err
	}

	// Retrieve and sanitize listen address from env
	listenAddr := os.Getenv("T0_HTTP_LISTEN_ADDR")
	listenAddr = strings.TrimSpace(listenAddr)

	// If listen address is not set, use default host and port
	if listenAddr == "" {
		listenAddr = ":3000"
	}

	// Print the URL where the app is running
	browsableURL := listenAddr
	if strings.HasPrefix(browsableURL, ":") {
		browsableURL = "localhost" + browsableURL

	}

	path, err := os.Getwd()
	if err != nil {
		s.Logger.Error(fmt.Sprintf("%w", err))
	}

	filePath := fmt.Sprintf("%s/static", path)

	// s.router.ServeFiles("/s3t/*filepath", http.Dir(filePath))
	fileServer1 := gzipped.FileServer(gzipped.Dir(filePath))
	mime.AddExtensionType(".js", "application/javascript")
	mime.AddExtensionType(".css", "text/css")
	mime.AddExtensionType(".gz", "application/gzip")

	mime.AddExtensionType(".png", "image/png")
	mime.AddExtensionType(".svg", "image/svg+xml")
	if len(o.StaticRoot) > 0 && !o.Embedded {

		fileServer2 := gzipped.FileServer(gzipped.Dir(o.StaticRoot[0]))

		my := func(c *gin.Context) {
			req := c.Request
			w := c.Writer
			ps := c.Params
			req.URL.Path = ps.ByName("filepath")
			if _, err := os.OpenFile(o.StaticRoot[0]+req.URL.Path, os.O_RDONLY, os.ModeAppend); err == nil {
				s.Logger.Debug(fmt.Sprintf("FileServer2::requesting: %s", o.StaticRoot[0]+req.URL.Path))
				fileServer2.ServeHTTP(w, req)

			} else {
				s.Logger.Debug(fmt.Sprintf("FileServer1::failed: %s", o.StaticRoot[0]+req.URL.Path))

				fileServer1.ServeHTTP(w, req)

			}

		}

		s.router.GET("/static/*filepath", my)
	} else if len(o.StaticRoot) > 1 && o.Embedded {

		fileServer2 := gzipped.FileServer(gzipped.Dir(o.StaticRoot[0]))
		contentStatic, _ := fs.Sub(o.EmbeddedDir, "static")

		fileServerEmbedded := gzipped.FileServer(gzipped.FS(contentStatic))
		s.Logger.Debug("using embedded ")
		fileServer3 := gzipped.FileServer(gzipped.Dir(o.StaticRoot[1]))
		s.router.GET("/static/*filepath", func(c *gin.Context) {
			req := c.Request
			w := c.Writer
			ps := c.Params
			req.URL.Path = ps.ByName("filepath")
			if _, err := os.OpenFile(o.StaticRoot[0]+req.URL.Path, os.O_RDONLY, os.ModeAppend); err == nil {
				s.Logger.Debug(fmt.Sprintf("FileServer2::requesting: %s", o.StaticRoot[0]+req.URL.Path))

				fileServer2.ServeHTTP(w, req)

			} else if _, err := os.OpenFile(o.StaticRoot[1]+req.URL.Path, os.O_RDONLY, os.ModeAppend); err == nil {
				s.Logger.Debug(fmt.Sprintf("FileServer3::requesting: %s", o.StaticRoot[1]+req.URL.Path))

				fileServer3.ServeHTTP(w, req)

			} else {
				s.Logger.Debug(fmt.Sprintf("FileServer1::failed: %s", o.StaticRoot[0]+req.URL.Path))

				req.URL.Path = ps.ByName("filepath")
				fileServerEmbedded.ServeHTTP(w, req)
			}

		})
	} else if len(o.StaticRoot) == 0 && o.Embedded {
		contentStatic, _ := fs.Sub(o.EmbeddedDir, "static")
		fileServerEmbedded := gzipped.FileServer(gzipped.FS(contentStatic))
		s.Logger.Debug("using embedded 2\n")
		contentGenerated, _ := fs.Sub(o.EmbeddedDir, "static/generated")
		fileServerEmbedded2 := http.StripPrefix("/", gzipped.FileServer(gzipped.FS(contentGenerated)))

		s.router.GET("/generated/*filepath", func(c *gin.Context) {
			req := c.Request
			w := c.Writer
			ps := c.Params
			req.URL.Path = ps.ByName("filepath")
			s.Logger.Debug(fmt.Sprintf("requesting : %s\n", req.URL.Path))
			fileServerEmbedded2.ServeHTTP(w, req)

		})

		s.router.GET("/assets/*filepath", func(c *gin.Context) {
			req := c.Request
			w := c.Writer
			ps := c.Params
			assets := servefiles.NewAssetHandler("./static/generated/css").StripOff(1).WithMaxAge(10 * 365 * 24 * time.Hour)
			req.URL.Path = ps.ByName("filepath")
			s.Logger.Debug(fmt.Sprintf("requesting : %s\n", req.URL.Path))
			assets.ServeHTTP(w, req)

		})

		s.router.GET("/static/*filepath", func(c *gin.Context) {
			req := c.Request
			w := c.Writer
			ps := c.Params
			req.URL.Path = ps.ByName("filepath")
			fileServerEmbedded.ServeHTTP(w, req)

		})

	} else {
		s.router.GET("/static/*filepath", func(c *gin.Context) {
			req := c.Request
			w := c.Writer
			ps := c.Params
			req.URL.Path = ps.ByName("filepath")
			fileServer1.ServeHTTP(w, req)
		})

	}

	s.Logger.Debug(fmt.Sprintf("Tndr0cean app running at http://%s\n", browsableURL))

	s.Logger.Debug(fmt.Sprintf("Tndr0cean serving files at %s%s\n file path is %s\n", path, "/static/", filePath))

	// Start the HTTP server
	return http.ListenAndServe(listenAddr, s.router)
}

func (s *Tndr0cean) add(method, path string, h Handler, plugs ...Plug) {
	s.router.Handle(method, path, s.makeHTTPRouterHandle(h, plugs...))
}

func (s *Tndr0cean) Get(path string, h Handler, plugs ...Plug) {
	s.add("GET", path, h, plugs...)
}

func (s *Tndr0cean) Post(path string, h Handler, plugs ...Plug) {
	s.add("POST", path, h, plugs...)
}

func (s *Tndr0cean) Put(path string, h Handler, plugs ...Plug) {
	s.add("PUT", path, h, plugs...)
}

func (s *Tndr0cean) Delete(path string, h Handler, plugs ...Plug) {
	s.add("DELETE", path, h, plugs...)
}

func (s *Tndr0cean) Head(path string, h Handler, plugs ...Plug) {
	s.add("HEAD", path, h, plugs...)
}

func (s *Tndr0cean) Options(path string, h Handler, plugs ...Plug) {
	s.add("OPTIONS", path, h, plugs...)
}

func (s *Tndr0cean) makeHTTPRouterHandle(h Handler, plugs ...Plug) gin.HandlerFunc {

	return func(c *gin.Context) {
		r := c.Request
		w := c.Writer
		params := c.Params
		ctx := newContext(w, r, params)
		keyList := make(map[string]bool)

		// Allows to retrieve global Data directly from the context, entries from Global Storage should be unique to be cached
		for j, v := range s.Global.Data {
			if !keyList[j] {
				keyList[j] = true
				ctx.Set(j, v)
			}
		}
		ctx.Set(s.Global.RepoName, s.Global.Repo)
		ctx.Set("LocalOntology", s.Global.Triples)
		ctx.Set("LocalOntologyPointer", s.Global.Triples)
		ctx.Set("LocalOntologyPrefixes", s.Global.Prefixes)

		for i := len(plugs) - 1; i >= 0; i-- {
			h = plugs[i](h)
		}
		for i := len(s.plugs) - 1; i >= 0; i-- {
			h = s.plugs[i](h)
		}
		h(ctx)
	}
}

func (c *Context) Ctx() *context.Context {
	ctx := context.WithValue(context.Background(), "request", c.Request)
	return &ctx

}

func defaultErrorHandler(err error, c *Context) error {
	log.Println("error", "err", err)
	return nil
}
