package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gomodule/redigo/redis"
	"golang.org/x/net/html"
	"io"
	"net/http"
	parse "net/url"
	"strings"
	"time"
)

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/", URLPreview).Methods("GET")
	router.NotFoundHandler = http.HandlerFunc(Custom404Handler)
	err := http.ListenAndServe(":8000", router)
	if err != nil {
		fmt.Print(err)
	}
}

// Custom404Handler handle 404 response
func Custom404Handler(w http.ResponseWriter, r *http.Request) {
	Respond(w, Message(nil, fmt.Sprintf("This url %s is not supported.", r.URL.Path), false))
}

// HandleURL check it and validations
func HandleURL(url string) (string, error) {
	if url == "" {
		msg := "You missed to set url query param."
		return msg, errors.New(msg)
	}
	u, err := parse.Parse(url)
	if err != nil {
        return err.Error(), err
	}
	if u.Scheme == "" {
		url = fmt.Sprintf("%s%s", "http://", url)
	} else if ! strings.HasPrefix(u.Scheme, "http") {
		msg := "URL schema must be http or https."
		return msg, errors.New(msg)
	}
	_, err = parse.ParseRequestURI(url)
	if err != nil {
		return err.Error(), err
	}
	return url, nil
}

// URLPreview function for main page
func URLPreview(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	url, err := HandleURL(v.Get("url"))
	if err != nil {
		Respond(w, Message(nil, url, false))
		return
	}
	pool := newPool()
    conn := pool.Get()
	defer conn.Close()
	meta := getStruct(conn, url)
    if meta == (HTMLMeta{}) {
		resp, err := http.Get(url)
		// handle the error if there is one
		if err != nil {
			Respond(w, Message(nil, err.Error(),false))
			return
		}
		// do this now so it won't be forgotten
		defer resp.Body.Close()
		meta = Extract(resp.Body)

		setStruct(conn, url, meta)
	} 
	data := map[string]interface{}{"url": url, "data": meta}
	Respond(w, Message(data, "",true))
}

// Message function structure response
func Message(data interface{}, message string, status bool) (map[string]interface{}) {
	now := time.Now()
	return map[string]interface{} {"data": data,"status" : status, "message" : message, "current_time": now.Unix()}
}

// Respond function send response as json
func Respond(w http.ResponseWriter, data map[string] interface{})  {
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
// HTMLMeta data to response
type HTMLMeta struct {
	Title       string
	Description string
	Image       string
	SiteName    string
	Icon        string
}


// Extract html meta tags
func Extract(resp io.Reader) (hm HTMLMeta) {
	z := html.NewTokenizer(resp)

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			if t.Data == "meta" {
				title, ok := extractMetaProperty(t, "og:title")
				if ok {
					hm.Title = title
				}

				desc, ok := extractMetaProperty(t, "og:description")
				if ok {
					hm.Description = desc
				}

				image, ok := extractMetaProperty(t, "og:image")
				if ok {
					hm.Image = image
				}

				siteName, ok := extractMetaProperty(t, "og:site_name")
				if ok {
					hm.SiteName = siteName
				}
			}
			if t.Data == "link" {
				icon, ok := extractIcon(t, "shortcut icon")
				if ok {
					hm.Icon = icon
				}
			}
		}
	}
}

func extractMetaProperty(t html.Token, prop string) (content string, ok bool) {
	for _, attr := range t.Attr {
		if attr.Key == "property" && attr.Val == prop {
			ok = true
		}

		if attr.Key == "content" {
			content = attr.Val
		}
	}

	return
}


func extractIcon(t html.Token, prop string) (content string, ok bool) {
	for _, attr := range t.Attr {
		if attr.Key == "rel" && attr.Val == prop {
			ok = true
		}

		if attr.Key == "href" {
			content = attr.Val
		}
	}

	return
}

func newPool() *redis.Pool {
	return &redis.Pool{
		// Maximum number of idle connections in the pool.
		MaxIdle: 80,
		// max number of connections
		MaxActive: 12000,
		// Dial is an application supplied function for creating and
		// configuring a connection.
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", ":6379")
			if err != nil {
				panic(err.Error())
			}
			return c, err
		},
	}
}

func setStruct(c redis.Conn, key string, data interface{}) error {

	const objectPrefix string = "url_preview:"

	// serialize User object to JSON
	json, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// SET object
	_, err = c.Do("SET", objectPrefix+key, json)
	if err != nil {
		return err
	}

	return nil
}

func getStruct(c redis.Conn, key string) interface{} {

	const objectPrefix string = "url_preview:"
	data := HTMLMeta{}

	s, err := redis.String(c.Do("GET", objectPrefix+key))
	if err == redis.ErrNil {
		return data
	} else if err != nil {
		return err
	}
	err = json.Unmarshal([]byte(s), &data)

	return data

}