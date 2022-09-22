package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Configuration represents configuration structure of the server
type Configuration struct {
	Port      int    `json:"port"`
	ServerKey string `json:"serverkey"`
	ServerCrt string `json:"servercrt"`
}

// Config is instance of Configruation
var Config Configuration

// version represents version of the server
var version string

// Record represent generic record
type Record map[string]interface{}

// helper function to generate series of records for given number of rows
func genNRecords(total int) []Record {
	var records []Record
	slice := make([]byte, 1024)
	size := runtime.Stack(slice, false)
	for i := 0; i < total; i++ {
		rec := make(Record)
		rec["id"] = i
		rec["data"] = slice[0:size]
		records = append(records, rec)
	}
	return records
}

// helper function to generate series of records totaling in size to given value
func genRecords(size string) ([]Record, error) {
	var records []Record
	var suffix string
	if strings.HasSuffix(size, "KB") {
		suffix = "KB"
	} else if strings.HasSuffix(size, "MB") {
		suffix = "MB"
	} else if strings.HasSuffix(size, "GB") {
		suffix = "GB"
	} else {
		return nil, errors.New("unsupported size, should be KB, MB or GB units")
	}
	arr := strings.Split(size, suffix)
	total, err := strconv.Atoi(arr[0])
	if err != nil {
		return nil, err
	}
	records = genNRecords(total)
	return records, nil
}

// HTTPError function dumpt http error to log and return back to user
func HTTPError(label, msg string, w http.ResponseWriter) {
	log.Println(label, msg)
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(msg))
}

// PayloadHandler provides API to test the payload
func PayloadHandler(w http.ResponseWriter, r *http.Request) {
	var latency int
	var size string
	var format string
	for k, values := range r.URL.Query() {
		if k == "latency" {
			v, err := strconv.Atoi(values[0])
			if err == nil {
				latency = v
			} else {
				msg := fmt.Sprintf("unable to convert latency value, error %v", err)
				HTTPError("ERROR", msg, w)
				return
			}
		} else if k == "size" {
			size = values[0]
		} else if k == "format" {
			format = values[0]
		}
	}
	if latency > 0 {
		time.Sleep(time.Duration(latency) * time.Second)
	}
	if format != "json" && format != "ndjson" {
		msg := fmt.Sprintf("unsupported format %s", format)
		HTTPError("ERROR", msg, w)
		return
	}

	records, err := genRecords(size)
	if err != nil {
		msg := fmt.Sprintf("unable to generate records, error %v", err)
		HTTPError("ERROR", msg, w)
		return
	}
	if format == "json" {
		data, err := json.Marshal(records)
		if err == nil {
			w.Write(data)
			return
		}
		msg := fmt.Sprintf("unable to marshal records, error %v", err)
		HTTPError("ERROR", msg, w)
		return
	} else if format == "ndjson" {
		for _, rec := range records {
			data, err := json.Marshal(rec)
			if err != nil {
				msg := fmt.Sprintf("unable to marshal records, error %v", err)
				HTTPError("ERROR", msg, w)
				return
			}
			w.Write(data)
			w.Write([]byte("\n"))
		}
	}
}

// RequestHandler handles incoming HTTP request
func RequestHandler(w http.ResponseWriter, r *http.Request) {
	log.Println(r.Method, r.URL, r.Proto, r.Host, r.RemoteAddr, r.Header)
	if r.Method == "GET" {
		// print out all request headers
		fmt.Fprintf(w, "%s %s %s \n", r.Method, r.URL, r.Proto)
		for k, v := range r.Header {
			h := strings.ToLower(k)
			if strings.Contains(h, "hmac") || strings.Contains(h, "cookie") {
				continue
			}
			fmt.Fprintf(w, "Header field %q, Value %q\n", k, v)
		}
		fmt.Fprintf(w, "Host = %q\n", r.Host)
		fmt.Fprintf(w, "RemoteAddr= %q\n", r.RemoteAddr)
		fmt.Fprintf(w, "\n\nFinding value of \"Accept\" %q\n", r.Header["Accept"])

		page := "Hello from Go\n"
		w.Write([]byte(page))
	} else {
		requestDump, err := httputil.DumpRequest(r, true)
		if err != nil {
			fmt.Fprint(w, err.Error())
		} else {
			fmt.Fprint(w, string(requestDump))
		}
	}
}

// helper function to parse the config
func parseConfig(configFile string) error {
	if configFile == "" {
		Config.Port = 8888
		return nil
	}
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		fmt.Println(err)
		return err
	}
	err = json.Unmarshal(data, &Config)
	if err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

// helper function to return version string of the server
func info() string {
	goVersion := runtime.Version()
	tstamp := time.Now().Format("2006-02-01")
	return fmt.Sprintf("httpgo git=%s go=%s date=%s", version, goVersion, tstamp)
}

// main function
func main() {
	var config string
	flag.StringVar(&config, "config", "", "configuration file")
	var version bool
	flag.BoolVar(&version, "version", false, "print version information about the server")
	flag.Parse()
	if version {
		fmt.Println(info())
		os.Exit(0)
	}
	// log time, filename, and line number
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	err := parseConfig(config)
	if err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/payload", PayloadHandler)
	http.HandleFunc("/", RequestHandler)
	if Config.ServerKey != "" && Config.ServerCrt != "" {
		server := &http.Server{
			Addr: fmt.Sprintf(":%d", Config.Port),
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
				//             ClientAuth: tls.RequestClientCert,
			},
		}
		err = server.ListenAndServeTLS(Config.ServerCrt, Config.ServerKey)
		if err != nil {
			fmt.Println("Unable to start the server", err)
		}
	} else {
		http.ListenAndServe(fmt.Sprintf(":%d", Config.Port), nil)
	}
}
