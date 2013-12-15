package main

import (
	"encoding/json"
	//"fmt"
	//"github.com/garyburd/redigo/redis"
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

func statsEndpoint(writer http.ResponseWriter, r *http.Request) {
	b, _ := json.Marshal(state)
	writer.Write(b)
}

func streamEndpoint(writer http.ResponseWriter, r *http.Request) {
	state.AddListener()
	defer state.RemoveListener()

	//uid_re := regexp.MustCompile(state.Config.StreamEndpointPattern)
	/*
		uids := uid_re.FindStringSubmatch(r.URL.Path)
		if len(uids) > 1 {
			uid := uids[1]
			defer log.Printf("Finishing request for %s.", uid)

			onConnection := make(chan redis.Conn)
			state.GetRedisConn(onConnection)
			c := <-onConnection
			defer c.Close()

			stream_key := fmt.Sprintf("stream:%s", uid)
			finished_key := fmt.Sprintf("stream:%s:finished", uid)
			expected_length_key := fmt.Sprintf("stream:%s:expected_length", uid)

			log.Printf("Received listener for %s.", uid)

			finished, _ := redis.String(c.Do("GET", finished_key))
			if len(finished) == 0 {
				//	The stream is still ongoing, or not yet started.
				exists, _ := redis.Bool(c.Do("EXISTS", stream_key))
				if exists {
					log.Printf("Opened stream for %s.", uid)
					//	Stream exists - let's read eagerly from it.

					//	Before we start, is there an expected length?

					send_ua := true
					for _, ua_disallow := range state.Config.UAsWithNoContentLength {
						if string, ok := ua_disallow.(string); ok {
							if strings.Contains(r.UserAgent(), string) {
								send_ua = false
								break
							}
						}
					}

					if send_ua {
						expected_length, err := redis.String(c.Do("GET", expected_length_key))
						if err == nil {
							log.Printf("Expected length set to %s bytes for %s.", expected_length, uid)
							writer.Header().Set("Content-Length", expected_length)
						} else {
							log.Printf("Expected length not set for %s.", uid)
						}
					} else {
						log.Printf("Not sending expected length for %s based on user agent.", uid)
					}

					read_elements := 0
					read_bytes := 0

					for {
						if finished, _ := redis.Bool(c.Do("EXISTS", finished_key)); finished {
							if llen, _ := redis.Int(c.Do("LLEN", stream_key)); llen == read_elements {
								log.Printf("Finished reading from stream after reading %d elements.", read_elements)
								break
							}
						} else if exists, _ := redis.Bool(c.Do("EXISTS", stream_key)); !exists {
							log.Printf("Stream removed. Closing connection after reading %d elements.", read_elements)
							break
						}

						reply, _ := redis.Values(c.Do("LRANGE", stream_key, read_elements, -1))
						read_elements += len(reply)

						for _, data := range reply {
							if bytes, ok := data.([]byte); ok {
								byte_count, _ := writer.Write(bytes)
								read_bytes += byte_count
							} else {
								log.Printf("Data is not byte array: %v", data)
							}
						}

						time.Sleep(time.Duration(state.Config.PollInterval) * time.Millisecond)
					}
					log.Printf("Ending stream for %s after sending %d bytes.", uid, read_bytes)
				} else {
					http.Error(writer, "This stream is not currently playing.", 404)
				}
			} else {
				log.Printf("Redirecting stream listener to %s.", finished)
				http.Redirect(writer, r, finished, 302)
			}
		} else {
			http.Error(writer, "No UID match found.", 400)
		}
	*/
}

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
	log.Println("Starting tilld.")
	rand.Seed(time.Now().UTC().UnixNano())

	state = NewState()

	usr1chan := make(chan os.Signal, 1)
	signal.Notify(usr1chan, syscall.SIGUSR1)
	go func() {
		for _ = range usr1chan {
			log.Printf("Reloading configuration from file.")
			state.Config.ReadFromJSON("./config.json")
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
	handler.HandleFunc(regexp.MustCompile("/stats\\.json"), statsEndpoint)

	err := http.ListenAndServe(":"+strconv.Itoa(state.Config.Port), handler)
	if err != nil {
		log.Printf("ListenAndServe failed:", err)
	}
}
