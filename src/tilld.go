package main

import (
	"encoding/json"
	//"fmt"

	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	//"strings"
	"syscall"
	"time"
)

var state State

func main() {
	logfile := "/var/log/streamer.log"
	if nlogfile := os.Getenv("LOGFILE"); nlogfile != "" {
		logfile = nlogfile
	}

	if logfile == "stdout" {
		log.SetOutput(os.Stderr)
	} else {
		logFile, err := os.OpenFile(logfile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0664)
		if err != nil {
			log.SetOutput(os.Stderr)
			log.Printf("Could not open regular log file: %v", err)
		} else {
			log.SetOutput(logFile)
		}
	}
	log.SetPrefix("tilld ")
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	state = NewState()

	log.Println("Starting tilld on port	", strconv.Itoa(state.Config.Port))
	rand.Seed(time.Now().UTC().UnixNano())

	usr1chan := make(chan os.Signal, 1)
	signal.Notify(usr1chan, syscall.SIGUSR1)
	go func() {
		for _ = range usr1chan {
			log.Printf("Reloading configuration from file.")

			//	TODO: Make sure we do all the other re-jiggering here.
			state.Config, _ = NewConfigFromJSON("./config.json")
		}
	}()

	termchan := make(chan os.Signal, 1)
	signal.Notify(termchan, os.Interrupt, syscall.SIGTERM)
	go func() {
		for _ = range termchan {
			state.Shutdown()
		}
	}()

	handler := &RegexpHandler{}
	handler.HandleFunc(regexp.MustCompile("^/api/v1/stats$"), StatsEndpoint)
	handler.HandleFunc(regexp.MustCompile("^/api/v1/object/"), ObjectGetPutEndpoint)

	err := http.ListenAndServe(":"+strconv.Itoa(state.Config.Port), handler)
	if err != nil {
		log.Printf("ListenAndServe failed:", err)
	}
}

func StatsEndpoint(writer http.ResponseWriter, r *http.Request) {
	b, _ := json.Marshal(state)
	writer.Write(b)
}

func ObjectGetPutEndpoint(writer http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		ObjectGetEndpoint(writer, r)
	case "POST":
		ObjectPostEndpoint(writer, r)
	case "PUT":
		ObjectPutEndpoint(writer, r)
	default:
		http.Error(writer, "Method not allowed.", 405)
	}
}

func GetID(writer http.ResponseWriter, r *http.Request) *string {
	id_re := regexp.MustCompile("^/api/v1/object/([a-zA-Z0-9_\\-.]+)$")
	ids := id_re.FindStringSubmatch(r.URL.Path)
	if len(ids) == 2 {
		return &ids[1]
	} else {
		http.Error(writer, "Malformed object ID. Must match regex /[a-zA-Z0-9_\\-.]+/.", 400)
		return nil
	}
}

func ObjectGetEndpoint(writer http.ResponseWriter, r *http.Request) {
	id := GetID(writer, r)
	if id != nil {
		for _, p := range state.Providers {
			if o, err := p.Get(*id); o != nil && err == nil {
				defer o.Close()

				size, err := o.GetSize()
				if err == nil && size != -1 {
					writer.Header().Set("Content-Length", strconv.FormatInt(size, 10))
				}
				metadata := o.GetBaseObject().Metadata
				if len(metadata) > 0 {
					writer.Header().Set("X-Till-Metadata", metadata)
				}

				for {
					read := 4096

					data, err := o.Read(read)
					if len(data) == 0 || err != nil {
						break
					} else {
						writer.Write(data)
					}

					if len(data) < read {
						break
					}
				}
				return
			}
		}
		http.Error(writer, "Failed to find object.", 404)
	}
}

func ObjectPostEndpoint(writer http.ResponseWriter, r *http.Request) {
	now := time.Now()
	var lifespan float64

	lifespan_s := r.Header.Get("X-Till-Lifespan")
	if len(lifespan_s) > 0 {
		if lifespan_s == "default" {
			//	TODO
		} else {
			var err error
			lifespan, err = strconv.ParseFloat(lifespan_s, 64)
			if err != nil || lifespan < 0 {
				http.Error(writer, "\"X-Till-Lifespan header is not a positive number or \\\"default\\\".\"", 400)
				return
			}
		}
	} else {
		http.Error(writer, "\"X-Till-Lifespan header must be provided.\"", 400)
		return
	}

	id := GetID(writer, r)

	if id != nil {
		obj := UploadObject{
			BaseObject: BaseObject{
				exists:     false,
				identifier: *id,
				provider:   nil,

				Expires:  now.Add(time.Duration(lifespan) * time.Second).Unix(),
				Metadata: r.Header.Get("X-Till-Metadata"),
			},
			reader: r.Body,
			size:   r.ContentLength,
		}
		defer obj.Close()

		//	TODO: Implement X-Till-Synchronous logic here.
		var synchronous bool
		synchronous_s := r.Header.Get("X-Till-Synchronous")
		if len(synchronous_s) > 0 {
			switch synchronous_s {
			case "0":
				synchronous = false
			case "1":
				synchronous = true
			default:
				http.Error(writer, "\"X-Till-Synchronous header is not exactly 0 or 1.\"", 400)
				return
			}
		} else {
			synchronous = false
		}

		//	TODO: Dispatch to all providers should happen at once, not sequentially.
		//	TODO: Dispatch to each provider should have a timeout associated with it.
		async := false
		success := false
		for _, p := range state.Providers {
			var err error
			if async {
				go func() {
					o, err := p.Put(&obj)
					if o != nil {
						defer o.Close()
					}
					if err != nil {
						log.Printf("Error while asynchronously saving object to %v: %v", p, err)
					}
				}()
			} else {
				var o Object
				o, err = p.Put(&obj)
				if o != nil {
					defer o.Close()
				}
			}
			if err != nil {
				log.Printf("Error while saving object to %v: %v", p, err)
			} else {
				success = true
				async = !synchronous
			}
		}

		if success && async {
			writer.WriteHeader(202)
		} else if success { // && !async
			writer.WriteHeader(201)
		} else { // !success && !async
			writer.WriteHeader(502)
		} // TODO: Implement 504
	}
}

func ObjectPutEndpoint(writer http.ResponseWriter, r *http.Request) {
	writer.Write([]byte("put"))
}
