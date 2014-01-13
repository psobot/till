package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var state State

func main() {
	log.SetPrefix("tilld " + strconv.Itoa(os.Getpid()) + "\t")
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	logfile := "/var/log/tilld.log"
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

	log.Printf("Initializing tilld...")

	state = NewState()
	log.Printf("Instance identifier: %v", state.Identifier)

	usr1chan := make(chan os.Signal, 1)
	signal.Notify(usr1chan, syscall.SIGUSR1)
	go func() {
		for _ = range usr1chan {
			log.Printf("Reloading configuration from file.")
			newstate := InitStateConfig(state)
			log.Printf("Replacing loaded configuration with configuration from disk.")
			state = newstate
		}
	}()

	termchan := make(chan os.Signal, 1)
	signal.Notify(termchan, os.Interrupt, syscall.SIGTERM)
	go func() {
		for _ = range termchan {
			log.Printf("Shutting down in response to SIGTERM.")
			os.Exit(0)
		}
	}()

	handler := &RegexpHandler{}
	handler.HandleFunc(regexp.MustCompile("^/api/v1/stats$"), StatsEndpoint)
	handler.HandleFunc(regexp.MustCompile("^/api/v1/object/"), ObjectGetPutEndpoint)
	handler.HandleFunc(regexp.MustCompile("^/api/v1/server/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"), TillRegistrationEndpoint)

	log.Printf("Starting tilld (pid %d) on port %d. Send SIGUSR1 to reload config.", os.Getpid(), state.Config.Port)

	fire_udp_port := os.Getenv("TEST_UDP_PORT")
	if fire_udp_port != "" {
		serverAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:"+fire_udp_port)
		con, _ := net.DialUDP("udp", nil, serverAddr)
		con.Write([]byte("connected"))
		con.Close()
	}

	//	This is a stupid race condition.
	go func() {
		time.Sleep(500 * time.Millisecond)
		for _, provider := range state.Providers {
			provider.OnServerUp()
		}
	}()

	if state.Config.Bind != "" {
		go func() {
			err := http.ListenAndServe(state.Config.Bind+":"+strconv.Itoa(state.Config.Port), handler)
			if err != nil {
				log.Printf("ListenAndServe on %v failed: %v", state.Config.Bind, err)
			}
		}()
	}

	err := http.ListenAndServe("127.0.0.1:"+strconv.Itoa(state.Config.Port), handler)
	if err != nil {
		log.Printf("ListenAndServe on %v failed: %v", state.Config.Bind, err)
	}
}

func StatsEndpoint(writer http.ResponseWriter, r *http.Request) {
	b, _ := json.Marshal(state)
	writer.Write(b)
}

func TillRegistrationEndpoint(writer http.ResponseWriter, r *http.Request) {
	components := strings.Split(r.URL.Path, "/")
	if len(components) > 1 {
		id := components[len(components)-1]
		address := r.Header.Get("X-Till-Address")
		lifespan_s := r.Header.Get("X-Till-Lifespan")

		lifespan := 86400.0
		if len(lifespan_s) > 0 {
			if lifespan_s != "default" {
				var err error
				lifespan, err = strconv.ParseFloat(lifespan_s, 64)
				if err != nil || lifespan < 0 {
					http.Error(writer, "\"X-Till-Lifespan must be a positive integer or 'default'.\"", 400)
					return
				}
			}
		}

		if _, exists := state.Servers[id]; !exists {
			go NotifyServer(state.Server, NewServer(id, address, int64(lifespan)))
		}

		data, err := json.Marshal(state.Server)
		if err != nil {
			http.Error(writer, "\"Could not marshal server state to return.\"", 500)
			return
		}
		writer.Write(data)
	}
}

func SendKnownServersTo(target Server) {
	servers := make(map[string]Server)
	state.metadataMutex.RLock()
	for id, known := range state.Servers {
		if id != target.Identifier && id != state.Identifier {
			servers[id] = known
		}
	}
	state.metadataMutex.RUnlock()

	for id, known := range servers {
		if id != target.Identifier && id != state.Identifier {
			NotifyServer(known, target)
		}
	}
}

func NotifyServer(source Server, target Server) {
	client := &http.Client{}

	if target.Identifier != "" {
		if target.Identifier != state.Identifier {
			log.Printf("Notifying %v of %v", target.Identifier, source.Identifier)
		} else {
			return
		}
	} else {
		if target.Address != state.Server.Address {
			log.Printf("Notifying %v of %v", target.Address, source.Identifier)
		} else {
			return
		}
	}

	req, err := http.NewRequest("POST", "http://"+target.Address+"/api/v1/server/"+source.Identifier, bytes.NewReader([]byte{}))
	if err != nil {
		log.Printf("Error making new outgoing Till request: %v", err)
	} else {
		req.Header.Add("X-Till-Address", source.Address)
		//	TODO: Implement me.
		//req.Header.Add("X-Till-Lifespan", source.Lifespan)
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error making new outgoing Till request: %v", err)
		} else {
			if resp.StatusCode == 200 {
				decoder := json.NewDecoder(resp.Body)
				var received Server
				err = decoder.Decode(&received)

				if err != nil {
					log.Printf("Could not decode data from other server: %v", err)
				} else {
					err = state.AddServer(received)
					if err == nil {
						SendKnownServersTo(received)
					}
				}
			} else {
				log.Printf("Response code from Till server not 200 - assuming down.")
				//	TODO: Remove server from list.
			}
			resp.Body.Close()
		}
	}
}

func ObjectGetPutEndpoint(writer http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		ObjectGetEndpoint(writer, r)
	case "POST":
		ObjectPostEndpoint(writer, r)
	case "PUT":
		ObjectUpdateEndpoint(writer, r)
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

func GetDefaultLifespan(id string) float64 {
	for p, l := range state.Config.LifespanPatterns {
		if p.MatchString(id) {
			return l
		}
	}
	return float64(state.Config.DefaultLifespan)
}

type RequestResult struct {
	Provider *Provider
	Object   *Object
	Error    error
	Timeout  bool
	NotFound bool
}

func (r *RequestResult) ForJSON() (string, map[string]string) {
	status := "OK"
	if r.Timeout {
		status = "TIMEOUT"
	} else if r.Error != nil {
		status = "ERROR"
	}

	data := map[string]string{"status": status}
	if r.Error != nil {
		data["error"] = r.Error.Error()
	}

	return (*(r.Provider)).Name(), data
}

func QueryProvider(id string, p Provider, result chan RequestResult) {
	obj, err := p.Get(id)

	defer func(obj Object) {
		if r := recover(); r != nil {
			if obj != nil {
				obj.Close()
			}
		}
	}(obj)

	result <- RequestResult{&p, &obj, err, false, obj == nil && err == nil}
}

func ObjectGetEndpoint(writer http.ResponseWriter, r *http.Request) {
	id := GetID(writer, r)
	if id != nil {
		var o RequestResult
		was_timeout := false

		//	TODO: Make me configurable
		timeout := 2000 // msec
		dispatched := 0
		received := 0
		successful := 0
		result := make(chan RequestResult)
		results := make(map[string]map[string]string)

		providers, _ := GetProviders(r, *id)
		for _, p := range providers {
			go QueryProvider(*id, p, result)
			dispatched++
		}
		endtime := time.Now().Add(time.Duration(timeout) * time.Millisecond)

	Join:
		for {
			select {
			case o = <-result:
				k, v := o.ForJSON()
				results[k] = v

				received++
				if o.Error == nil && !o.NotFound {
					successful++
					break Join
				}

				if received == dispatched {
					break Join
				}

			case <-time.After(endtime.Sub(time.Now())):
				log.Printf("Timeout exceeded when getting object %s.", *id)
				was_timeout = true
				break Join
			}
		}

		close(result)

		if !o.NotFound && o.Error == nil && o.Object != nil {
			obj := *(o.Object)
			defer obj.Close()

			size, err := obj.GetSize()
			if err == nil && size != -1 {
				writer.Header().Set("Content-Length", strconv.FormatInt(size, 10))
			}
			metadata := obj.GetBaseObject().Metadata
			if len(metadata) > 0 {
				writer.Header().Set("X-Till-Metadata", metadata)
			}

			data := make([]byte, 4096)
			for {
				length, err := obj.Read(data)
				if length == 0 || (err != nil && err != io.EOF) {
					break
				} else {
					writer.Write(data[0:length])
				}

				if err == io.EOF {
					break
				}
			}
			return
		} else if was_timeout {
			providers, _ := GetProviders(r, *id)
			for _, p := range providers {
				if _, exists := results[p.Name()]; !exists {
					results[p.Name()] = map[string]string{
						"status":     "TIMEOUT",
						"timeout_ms": strconv.FormatInt(int64(timeout), 10),
					}
				}
			}

			jsondata, err := json.Marshal(results)
			if err != nil {
				log.Printf("Could not marshal error result data: %v", err)
				http.Error(writer, "\"Failed to find object within given time.\"", 504)
			} else {
				http.Error(writer, string(jsondata), 504)
			}
		} else if successful < dispatched && !o.NotFound {
			jsondata, err := json.Marshal(results)
			if err != nil {
				log.Printf("Could not marshal error result data: %v", err)
				http.Error(writer, "\"Upstream provider failed to query object.\"", 503)
			} else {
				http.Error(writer, string(jsondata), 503)
			}
		} else {
			http.Error(writer, "\"Failed to find object.\"", 404)
		}
	}
}

func GetProviders(r *http.Request, id string) (map[string]Provider, error) {
	var err error
	target_providers := make(map[string]Provider)

	for name, provider := range state.Providers {
		if provider.AcceptsKey(id) {
			target_providers[name] = provider
		}
	}

	specified_providers := r.Header.Get("X-Till-Providers")
	if len(specified_providers) > 0 {
		provider_list := strings.Split(specified_providers, ",")

		target_providers = make(map[string]Provider)
		for _, s := range provider_list {
			provider, ok := state.Providers[s]
			if ok {
				if _, exists := target_providers[s]; !exists {
					target_providers[s] = provider
				}
			} else {
				log.Printf("Specified provider \"%v\" not found.", s)
				err = errors.New(fmt.Sprintf("Specified provider \"%v\" not found.", s))
			}
		}
	}

	return target_providers, err
}

func GetSynchronized(r *http.Request) (bool, error) {
	synchronous_s := r.Header.Get("X-Till-Synchronized")
	if len(synchronous_s) > 0 {
		switch synchronous_s {
		case "0":
			return false, nil
		case "1":
			return true, nil
		default:
			return false, errors.New("X-Till-Synchronized header is not exactly 0 or 1.")
		}
	} else {
		return false, nil
	}
}

func GetLifespan(id string, r *http.Request) (float64, error) {
	lifespan_s := r.Header.Get("X-Till-Lifespan")
	if len(lifespan_s) > 0 {
		if lifespan_s == "default" {
			return GetDefaultLifespan(id), nil
		} else {
			lifespan, err := strconv.ParseFloat(lifespan_s, 64)
			if err != nil || lifespan < 0 {
				return -1, errors.New("X-Till-Lifespan header is not a positive integer.")
			} else {
				return lifespan, nil
			}
		}
	} else {
		return -1, errors.New("X-Till-Lifespan header must be provided.")
	}
}

func ObjectPostEndpoint(writer http.ResponseWriter, r *http.Request) {
	now := time.Now()
	id := GetID(writer, r)

	if id != nil {
		lifespan, err := GetLifespan(*id, r)
		if err != nil {
			http.Error(writer, "\""+err.Error()+"\"", 400)
			return
		}

		buf := NewFullyBufferedReader(r.Body)

		bo := BaseObject{
			exists:     false,
			identifier: *id,
			provider:   nil,

			Expires:  now.Add(time.Duration(lifespan) * time.Second).Unix(),
			Metadata: r.Header.Get("X-Till-Metadata"),
		}

		synchronous, err := GetSynchronized(r)
		if err != nil {
			http.Error(writer, "\""+err.Error()+"\"", 400)
			return
		}

		//	TODO: Dispatch to each provider should have a timeout associated with it.
		var o RequestResult

		was_timeout := false

		timeout := 1000 // msec
		dispatched := 0
		received := 0
		successful := 0
		result := make(chan RequestResult)

		results := make(map[string]map[string]string)

		providers, provider_error := GetProviders(r, *id)
		for _, p := range providers {
			go SaveObject(p, bo, buf, r.ContentLength, result)
			dispatched++
		}

		endtime := time.Now().Add(time.Duration(timeout) * time.Millisecond)

		if dispatched > 0 {
		Join:
			for {
				select {
				case o = <-result:
					k, v := o.ForJSON()
					results[k] = v

					received++
					if o.Error == nil {
						successful++

						if !synchronous || received == dispatched {
							break Join
						}
					} else if received == dispatched {
						break Join
					}

				case <-time.After(endtime.Sub(time.Now())):
					if synchronous {
						log.Printf("Timeout exceeded when posting object %s.", *id)
						was_timeout = true
					}
					break Join
				}
			}
		}

		if dispatched == 0 && provider_error != nil {
			jsondata, _ := json.Marshal(provider_error.Error())
			http.Error(writer, string(jsondata), 404)
			return
		}

		if successful > 0 && (!synchronous || successful < dispatched) {
			writer.WriteHeader(202)
		} else if successful > 0 { // && synchronous
			writer.WriteHeader(201)
		} else if was_timeout {
			providers, _ := GetProviders(r, *id)
			for _, p := range providers {
				if _, exists := results[p.Name()]; !exists {
					results[p.Name()] = map[string]string{
						"status":     "TIMEOUT",
						"timeout_ms": strconv.FormatInt(int64(timeout), 10),
					}
				}
			}

			jsondata, err := json.Marshal(results)
			if err != nil {
				log.Printf("Could not marshal error result data: %v", err)
				http.Error(writer, "\"Failed to find object within given time.\"", 504)
			} else {
				http.Error(writer, string(jsondata), 504)
			}
		} else if dispatched == 0 {
			http.Error(writer, "\"No providers could handle the provided key. Ensure that whitelists are appropriately configured.\"", 404)
		} else {
			jsondata, err := json.Marshal(results)
			if err != nil {
				log.Printf("Could not marshal error result data: %v", err)
				http.Error(writer, "\"Failed to find object due to upstream errors.\"", 502)
			} else {
				http.Error(writer, string(jsondata), 502)
			}
		}
	}
}

func SaveObject(p Provider, bo BaseObject, buf *FullyBufferedReader, size int64, result chan RequestResult) {
	obj := UploadObject{
		BaseObject: bo,
		reader:     buf.Reader(),
		size:       size,
	}
	defer obj.Close()
	o, err := p.Put(&obj)
	if o != nil {
		o.Close()
	}
	if err != nil {
		log.Printf("Error saving object %v to %v: %v", bo.identifier, p, err)
		result <- RequestResult{
			Provider: &p,
			Object:   &o,
			Error:    err,
			Timeout:  false,
			NotFound: false,
		}
	} else {
		result <- RequestResult{
			Provider: &p,
			Object:   &o,
			Error:    nil,
			Timeout:  false,
			NotFound: false,
		}
	}
}

func UpdateObject(p Provider, bo BaseObject, result chan *Object) {
	obj := UploadObject{BaseObject: bo}
	defer obj.Close()
	o, err := p.Update(&obj)
	if err != nil {
		log.Printf("Error updating object %v to %v: %v", bo.identifier, p, err)
		result <- nil
		if o != nil {
			o.Close()
		}
	} else {
		result <- &o
	}
}

func ObjectUpdateEndpoint(writer http.ResponseWriter, r *http.Request) {
	now := time.Now()

	id := GetID(writer, r)

	if id != nil {
		lifespan, err := GetLifespan(*id, r)
		if err != nil {
			http.Error(writer, "\""+err.Error()+"\"", 400)
			return
		}

		bo := BaseObject{
			identifier: *id,
			provider:   nil,
			Expires:    now.Add(time.Duration(lifespan) * time.Second).Unix(),
		}

		synchronous, err := GetSynchronized(r)
		if err != nil {
			http.Error(writer, "\"X-Till-Synchronized header is not exactly 0 or 1.\"", 400)
			return
		}

		//	TODO: Dispatch to all providers should happen at once, not sequentially.
		//	TODO: Dispatch to each provider should have a timeout associated with it.
		was_timeout := false

		timeout := 1000 // msec
		dispatched := 0
		received := 0
		successful := 0
		result := make(chan *Object)

		providers, _ := GetProviders(r, *id)
		for _, p := range providers {
			go UpdateObject(p, bo, result)
			dispatched++
		}

		endtime := time.Now().Add(time.Duration(timeout) * time.Millisecond)

	Join:
		for {
			select {
			case o := <-result:
				received++
				if o != nil {
					successful++

					if !synchronous || received == dispatched {
						break Join
					}
				} else if received == dispatched {
					break Join
				}

			case <-time.After(endtime.Sub(time.Now())):
				if synchronous {
					log.Printf("Timeout exceeded when posting object %s.", *id)
					was_timeout = true
				}
				break Join
			}
		}

		if successful > 0 && (!synchronous || successful < dispatched) {
			writer.WriteHeader(202)
		} else if successful > 0 { // && synchronous
			writer.WriteHeader(201)
		} else if was_timeout {
			writer.WriteHeader(504)
		} else {
			writer.WriteHeader(502)
		}
	}
}
