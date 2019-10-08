package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/oschwald/geoip2-golang"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"syscall"
)

const FileDescLimit = 100000
var MessageData = make(map[string][]byte)
var PortMappings map[int]string
var geoipDB *geoip2.Reader

var (
	nConnectFlag = flag.Int("concurrent", 5, "Number of concurrent connections")
	formatFlag   = flag.String("format", "json", "Output format for responses ('ascii', 'hex', json, or 'base64')")
	timeoutFlag  = flag.Int("timeout", 4, "Seconds to wait for each host to respond")
	geoipDbFlag = flag.String("geoip", "", "Path to geoip db")
	dataFileFlag = flag.String("data", "", "Directory containing protocol messages to send to responsive hosts ('%s' will be replaced with host IP)")
	portMappingsFlag = flag.String("config", "", "Json file containing data file port mappings")
)

// Before running main, parse flags and load message data, if applicable
func init() {
	flag.Parse()

	dir, err := filepath.Abs(*geoipDbFlag)
	if err != nil {
		log.Fatal(err)
	}

	_geoipDB, err := geoip2.Open(dir)
	geoipDB = _geoipDB
	if err != nil {
		panic(err)
	}

	configFile, err := os.Open(*portMappingsFlag)
	if err != nil {
		panic(err)
	}
	defer configFile.Close()

	configFileBytes, err := ioutil.ReadAll(configFile)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(configFileBytes, &PortMappings)
	if err != nil {
		panic(err)
	}

	if *dataFileFlag != "" {
		dir, err := ioutil.ReadDir(*dataFileFlag)
		if err != nil {
			panic(err)
		}

		for _, dataFile := range dir {
			dataFileName := dataFile.Name()
			fi, err := os.Open(path.Join(*dataFileFlag, dataFileName))
			if err != nil {
				panic(err)
			}

			buf := make([]byte, 1024)
			n, err := fi.Read(buf)
			MessageData[dataFileName] = buf[0:n]
			if err != nil && err != io.EOF {
				panic(err)
			}
			_ = fi.Close()
		}
	}

	// Increase file descriptor limit
	rlimit := syscall.Rlimit{Max: uint64(FileDescLimit), Cur: uint64(FileDescLimit)}
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "[error] cannot set rlimit: %s", err)
	}
}

type ProbeResult struct {
	Addr            string // address of remote host
	Port            int    // connected port of remote host
	ProbbedProtocol string // probed for protocol
	Data            []byte // data returned from the host, if successful
	Err             string // error, if any
}

func main() {
	addrChan := make(chan JsonRawIpPort, *nConnectFlag) // pass addresses to grabbers
	resultChan := make(chan ProbeResult, *nConnectFlag) // grabbers send results to output
	doneChan := make(chan int, *nConnectFlag)           // let grabbers signal completion

	// Start grabbers and output thread
	go Output(resultChan, doneChan)
	for i := 0; i < *nConnectFlag; i++ {
		go GrabBanners(addrChan, resultChan, doneChan)
	}

	defer func() {
		close(addrChan)
		close(resultChan)
		<-doneChan
	}()

	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		msg := sc.Text()
		addr, err := decodeJson(string(msg))
		if err != nil {
			fmt.Fprintf(os.Stderr, "[error] cannot decode payload %s\n", msg)
			continue
		}

		addrChan <- addr
	}
}
