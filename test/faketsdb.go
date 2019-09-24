// Binary faketsdb is a very simple fake instance of opentsdb, for integration
// testing of Geras.
package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

var fakeMetrics = []string{}

func main() {
	for i := 0; i < 1000000; i++ {
		n := 'a' + (i % 26)
		fakeMetrics = append(fakeMetrics, fmt.Sprintf("test.%c.%d", n, i))
	}

	http.HandleFunc("/api/suggest", jsonWrap(bodyReader(suggest)))
	http.HandleFunc("/api/version", jsonWrap(version))
	http.HandleFunc("/api/query", jsonWrap(bodyReader(query)))
	http.HandleFunc("/api/query/last", jsonWrap(bodyReader(queryLast)))
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func jsonWrap(f func (r *http.Request) interface{}) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL)
		w.Header().Set("Content-Type", "application/json")
		result := f(r)
		b, err := json.Marshal(result)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(b)
	}
}

func bodyReader(f func (r *http.Request, body []byte) interface{}) func (r *http.Request) interface{} {
	return func (r *http.Request) interface{} {
		if r.Method == "POST" {
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.Printf("bodyReader: %v", err)
				return nil
			}
			r.Body.Close()
			log.Printf("body: %s", string(body))
			return f(r, body)
		} else {
			return f(r, nil)
		}
	}
}

func version(r *http.Request) interface{} {
	return map[string]string{
		"version": "faketsdb go",
	}
}

func suggest(r *http.Request, body []byte) interface{} {
	return fakeMetrics
}

func query(r *http.Request, body []byte) interface{} {
	return []string{}
}

func queryLast(r *http.Request, body []byte) interface{} {
	return []string{}
}
