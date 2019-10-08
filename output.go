package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"strconv"
	"strings"
)

type JsonOutput struct {
	Addr            string
	Port            int
	Protocol        string
	ProbbedProtocol string
	Country         string
	City            string
	Data            string
	Metadata        interface{}
	Err             string
}

type Header map[string][]string
type HttpMetadata struct {
	Header     Header
	Status     string
	StatusCode int
	Protocol   string
	ProtoMajor int
	ProtoMinor int
}

func getHttpMetadata(raw []byte) (*HttpMetadata, error) {
	bufReader := bufio.NewReader(bytes.NewReader(raw))
	tp := textproto.NewReader(bufReader)

	metadata := &HttpMetadata{}

	// Parse the first line of the response.
	line, err := tp.ReadLine()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}

	if i := strings.IndexByte(line, ' '); i == -1 {
		return nil, errors.New("not HTTP")
	} else {
		metadata.Protocol = line[:i]
		metadata.Status = strings.TrimLeft(line[i+1:], " ")
	}

	statusCode := metadata.Status
	if i := strings.IndexByte(metadata.Status, ' '); i != -1 {
		statusCode = metadata.Status[:i]
	}
	if len(statusCode) != 3 {
		return nil, errors.New("bad HTTP status code")
	}

	metadata.StatusCode, err = strconv.Atoi(statusCode)
	if err != nil || metadata.StatusCode < 0 {
		return nil, errors.New("bad HTTP status code")
	}

	var ok bool
	if metadata.ProtoMajor, metadata.ProtoMinor, ok = http.ParseHTTPVersion(metadata.Protocol); !ok {
		return nil, errors.New("bad HTTP version")
	}

	// Parse the response headers.
	mimeHeader, err := tp.ReadMIMEHeader()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}

	metadata.Header = Header(mimeHeader)
	return metadata, nil
}

type GeoipLookup struct {
	Country string
	City    string
}

// make a lookup on geoip database and format it to internal look metadata shape.
func lookupOnGeoip2DB(addr string) *GeoipLookup {
	ip := net.ParseIP(addr)
	result := &GeoipLookup{}

	if country, err := geoipDB.Country(ip); err == nil {
		result.Country = country.Country.IsoCode
	}

	if city, err := geoipDB.City(ip); err == nil {
		result.City = city.City.Names["en"]
	}

	return result
}

func probeResultToJsonString(result ProbeResult) string {
	protocol := "unknown"
	meta, _ := getHttpMetadata(result.Data)
	if meta != nil {
		protocol = meta.Protocol
	}

	geoipLookup := lookupOnGeoip2DB(result.Addr)

	js, err := json.Marshal(JsonOutput{
		result.Addr,
		result.Port,
		protocol,
		result.ProbbedProtocol,
		geoipLookup.Country,
		geoipLookup.City,
		string(result.Data),
		meta,
		result.Err,
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "[error] cannot json %s: %s\n", result.Addr, result.Err)
		return ""
	}

	return string(js) + "\n"
}

// Read resultStructs from resultChan, print output, and maintain
// status counters.  Writes to doneChan when complete.
func Output(resultChan chan ProbeResult, doneChan chan int) {
	ok, errorsCount := 0, 0

	for result := range resultChan {
		var output string

		switch *formatFlag {
		case "hex":
			output = fmt.Sprintf("%s: %s\n", result.Addr,
				hex.EncodeToString(result.Data))
		case "base64":
			output = fmt.Sprintf("%s: %s\n", result.Addr,
				base64.StdEncoding.EncodeToString(result.Data))
		case "ascii":
			output = fmt.Sprintf("%s: %s\n", result.Addr,
				string(result.Data))
		default:
			output = fmt.Sprintf(probeResultToJsonString(result))
		}

		fmt.Printf(output)

		if result.Err == "" {
			ok++
		} else {
			errorsCount++
		}
	}

	fmt.Fprintf(os.Stderr, "Complete (OK=%d, errorsCount=%d)\n",
		ok, errorsCount)

	doneChan <- 1
}
