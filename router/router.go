package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/julienschmidt/httprouter"
	t1 "github.com/senforsce/tndr"
)

type Plug func(Handler) Handler

type Handler func(c *Context) error

type ErrorHandler func(error, *Context) error

type Context struct {
	Response http.ResponseWriter
	Request  *http.Request
	ctx      context.Context
	params   httprouter.Params
}

func newContext(w http.ResponseWriter, r *http.Request, params httprouter.Params) *Context {
	return &Context{
		Response: w,
		Request:  r,
		ctx:      context.Background(),
		params:   params,
	}
}

func (c *Context) Param(name string) string {
	return c.params.ByName(name)
}

func (c *Context) Query(name string) string {
	return c.Request.URL.Query().Get(name)
}

func (c *Context) FormValue(name string) string {
	return c.Request.FormValue(name)
}

func (c *Context) Render(component t1.Component) error {
	return component.Render(c.ctx, c.Response)
}

func (c *Context) Redirect(url string, code int) error {
	if code < http.StatusMultipleChoices || code > http.StatusTemporaryRedirect {
		return errors.New("invalid redirect code")
	}
	http.Redirect(c.Response, c.Request, url, code)
	return nil
}

func (c *Context) JSON(status int, v any) error {
	c.Response.Header().Set("Content-Type", "application/json")
	c.Response.WriteHeader(status)
	return json.NewDecoder(c.Request.Body).Decode(&v)
}

func (c *Context) Text(status int, t string) error {
	c.Response.Header().Set("Content-Type", "text/plain")
	c.Response.WriteHeader(status)
	_, err := c.Response.Write([]byte(t))
	return err
}

func (c *Context) Set(key string, value any) {
	c.ctx = context.WithValue(c.ctx, key, value)
}

func (c *Context) Get(key string) any {
	res := c.ctx.Value(key)

	if res != nil {
		return res
	}

	return key
}

type Tndr0cean struct {
	ErrorHandler ErrorHandler
	router       *httprouter.Router
	plugs        []Plug
	Global       GlobalStorage
}

func New() *Tndr0cean {
	return &Tndr0cean{
		router:       httprouter.New(),
		ErrorHandler: defaultErrorHandler,
		Global: GlobalStorage{
			Data: make(map[string]string),
		},
	}
}

// initialized by the first start
type GlobalStorage struct {
	Data map[string]string
}

func (t0 *Tndr0cean) S(key string, value string) {
	t0.Global.Data[key] = value
}

func (t0 *Tndr0cean) G(key string) string {
	return t0.Global.Data[key]
}

type methodNotAllowedHandler struct {
	handler Handler
}

func (h methodNotAllowedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := newContext(w, r, httprouter.Params{})
	h.handler(ctx)
}

func (s *Tndr0cean) MethodNotAllowed(h Handler) {
	s.router.MethodNotAllowed = methodNotAllowedHandler{h}
}

func (s *Tndr0cean) Plug(plugs ...Plug) {
	s.plugs = append(s.plugs, plugs...)
}

func (s *Tndr0cean) Start() error {
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
		log.Println(err)
	}

	filePath := fmt.Sprintf("%s/static", path)

	s.router.ServeFiles("/static/*filepath", http.Dir(filePath))

	fmt.Printf("Tndr0cean app running at http://%s\n", browsableURL)

	fmt.Printf("Tndr0cean serving files at %s%s\n file path is %s\n", path, "/static/", filePath)

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

func (s *Tndr0cean) makeHTTPRouterHandle(h Handler, plugs ...Plug) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		ctx := newContext(w, r, params)
		for j, v := range s.Global.Data {
			ctx.Set(j, v)
		}
		for i := len(plugs) - 1; i >= 0; i-- {
			h = plugs[i](h)
		}
		for i := len(s.plugs) - 1; i >= 0; i-- {
			h = s.plugs[i](h)
		}
		if err := h(ctx); err != nil {
			// todo: handle the error from the error handler huh?
			s.ErrorHandler(err, ctx)
		}
	}
}

func defaultErrorHandler(err error, c *Context) error {
	log.Println("error", "err", err)
	return nil
}
