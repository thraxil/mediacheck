package main // import "github.com/thraxil/mediacheck"

import (
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/net/html"
)

var TIMEOUT = 1000 * time.Millisecond
var wg sync.WaitGroup

type failure struct {
	URL *url.URL
	Err error
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("must specify a URL")
	}
	fetchUrl := os.Args[1]
	u, err := url.Parse(fetchUrl)
	if err != nil {
		log.WithFields(log.Fields{
			"URL": fetchUrl,
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
	ctx, cancel := context.WithTimeout(context.Background(), TIMEOUT)
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
		return getImgSrc(n)
	case n.Data == "link":
		return getLinkHref(n)
	case n.Data == "script":
		return getScriptSrc(n)
	case n.Data == "video":
		return getVideoSrc(n)
	}
	return false, nil
}

func getImgSrc(n *html.Node) (bool, *url.URL) {
	for _, a := range n.Attr {
		if a.Key == "src" {
			url, err := url.Parse(a.Val)
			if err == nil {
				return true, url
			}
		}
	}
	return false, nil
}

func getScriptSrc(n *html.Node) (bool, *url.URL) {
	for _, a := range n.Attr {
		if a.Key == "src" {
			url, err := url.Parse(a.Val)
			if err == nil {
				return true, url
			}
		}
	}
	return false, nil
}

func getLinkHref(n *html.Node) (bool, *url.URL) {
	for _, a := range n.Attr {
		if a.Key == "href" {
			url, err := url.Parse(a.Val)
			if err == nil {
				return true, url
			}
		}
	}
	return false, nil
}

func getVideoSrc(n *html.Node) (bool, *url.URL) {
	for _, a := range n.Attr {
		if a.Key == "src" {
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
