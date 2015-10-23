package main // import "github.com/thraxil/mediacheck"

import (
	"errors"
	"flag"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/net/html"
)

var wg sync.WaitGroup

type failure struct {
	URL *url.URL
	Err error
}

func main() {
	var URL = flag.String("url", "", "URL to check")
	var timeout = flag.Int("timeout", 3000, "timeout (ms)")
	var loglevel = flag.String("log-level", "error", "log level: info/warn/error")
	var logformat = flag.String("log-format", "text", "log format: text or json")
	flag.Parse()

	if *loglevel == "info" {
		log.SetLevel(log.InfoLevel)
	}
	if *loglevel == "warn" {
		log.SetLevel(log.WarnLevel)
	}
	if *loglevel == "error" {
		log.SetLevel(log.ErrorLevel)
	}

	if *logformat == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	}

	if *URL == "" {
		log.Fatal("must specify a URL")
	}
	u, err := url.Parse(*URL)
	if err != nil {
		log.WithFields(log.Fields{
			"URL": *URL,
		}).Fatal(err)
	}
	if !u.IsAbs() {
		log.WithFields(log.Fields{"URL": u.String()}).Fatal("must be an absolute URL")
	}
	log.WithFields(
		log.Fields{
			"URL":    u.String(),
			"Scheme": u.Scheme,
			"Host":   u.Host,
			"Path":   u.Path,
		}).Info("fetching")
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*time.Duration(*timeout))
	defer cancel()
	status, result, err := fetchURL(ctx, u)
	if err != nil {
		log.Fatal(err)
	}
	if status != "200 OK" {
		log.WithFields(log.Fields{
			status: status,
		}).Fatal("bad response status")
	}
	log.WithFields(log.Fields{}).Info("retrieved page")

	mediaUrls := extractMedia(string(result))
	log.WithFields(log.Fields{
		"number": len(mediaUrls),
	}).Info("extracted media URLs")

	failures := make([]failure, 0)
	fail := make(chan failure, 0)

	go func() {
		// this is a leaked goroutine, but it's a one-shot commandline
		// app, so we just tolerate it
		for f := range fail {
			failures = append(failures, f)
		}
	}()

	for _, mediaURL := range mediaUrls {
		absURL := u.ResolveReference(mediaURL)
		if u.Scheme == "https" && absURL.Scheme != "https" {
			log.WithFields(log.Fields{"url": absURL.String()}).Fatal("HTTP/S mixed content error")
		}
		wg.Add(1)
		go func() {
			res := checkMedia(ctx, absURL)
			if res != nil {
				fail <- failure{absURL, res}
			}
		}()
	}
	wg.Wait()
	if len(failures) == 0 {
		log.Info("OK")
	} else {
		for _, f := range failures {
			log.WithFields(log.Fields{
				"URL":   f.URL.String(),
				"Error": f.Err,
			}).Error("Error fetching media")
		}
		log.Fatal("NOT OK")
	}
}

func fetchURL(ctx context.Context, u *url.URL) (string, []byte, error) {
	tr := &http.Transport{}
	client := http.Client{Transport: tr}
	c := make(chan struct {
		r   *http.Response
		err error
	}, 1)
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return "", nil, err
	}
	go func() {
		resp, err := client.Do(req)
		pack := struct {
			r   *http.Response
			err error
		}{resp, err}
		c <- pack
	}()

	select {
	case <-ctx.Done():
		tr.CancelRequest(req)
		<-c // wait for client.Do
		log.Info("cancel the context")
		return "", nil, ctx.Err()
	case ok := <-c:
		err := ok.err
		resp := ok.r
		if err != nil {
			log.Info(err)
			return "", nil, err
		}
		defer resp.Body.Close()
		b, _ := ioutil.ReadAll(resp.Body)
		return resp.Status, b, nil
	}
	// should never actually reach here
	return "", nil, nil
}

func extractMedia(s string) []*url.URL {
	r := strings.NewReader(s)
	doc, err := html.Parse(r)

	if err != nil {
		log.WithFields(log.Fields{"error": err}).Fatal("parse failed")
	}
	urls := make([]*url.URL, 0)
	var f func(*html.Node)

	f = func(n *html.Node) {
		found, url := getMediaURL(n)
		if found {
			urls = append(urls, url)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	return urls
}

func getMediaURL(n *html.Node) (bool, *url.URL) {
	if n.Type != html.ElementNode {
		return false, nil
	}
	switch {
	case n.Data == "img":
		return getElementSrc(n)
	case n.Data == "link":
		return getElementHref(n)
	case n.Data == "script":
		return getElementSrc(n)
	case n.Data == "video":
		return getElementSrc(n)
	case n.Data == "source":
		return getElementSrc(n)
	case n.Data == "track":
		return getElementSrc(n)
	case n.Data == "iframe":
		return getElementSrc(n)				
	}
	return false, nil
}

func getElementSrc(n *html.Node) (bool, *url.URL) {
	return getElementAttr(n, "src")
}

func getElementHref(n *html.Node) (bool, *url.URL) {
	return getElementAttr(n, "href")
}

func getElementAttr(n *html.Node, key string) (bool, *url.URL) {
	for _, a := range n.Attr {
		if a.Key == key {
			url, err := url.Parse(a.Val)
			if err == nil {
				return true, url
			}
		}
	}
	return false, nil
}

func checkMedia(ctx context.Context, u *url.URL) error {
	defer wg.Done()
	log.WithFields(log.Fields{
		"url": u.String(),
	}).Info("checking media URL")
	tr := &http.Transport{}
	client := http.Client{Transport: tr}
	c := make(chan struct {
		r   *http.Response
		err error
	}, 1)
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		log.WithFields(log.Fields{"url": u.String()}).Error(err)
		return err
	}
	go func() {
		resp, err := client.Do(req)
		pack := struct {
			r   *http.Response
			err error
		}{resp, err}
		c <- pack
	}()

	select {
	case <-ctx.Done():
		tr.CancelRequest(req)
		<-c // wait for client.Do
		log.WithFields(log.Fields{"url": u.String()}).Error(ctx.Err())
		return ctx.Err()
	case ok := <-c:
		err := ok.err
		resp := ok.r
		if err != nil {
			log.WithFields(log.Fields{"url": u.String()}).Error(err)
			return err
		}
		if resp.Status != "200 OK" {
			log.WithFields(log.Fields{"url": u.String(), "status": resp.Status}).Error("not a 200")
			return errors.New("bad status")
		}
		return nil
	}
	// should never actually reach here
	return nil
}
