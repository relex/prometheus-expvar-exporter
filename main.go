package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/exp/maps"
)

var (
	configAddr    = flag.String("addr", "127.0.0.1:8000", "Address to listen proxy requests, e.g. 0.0.0.0:8000.")
	configTimeout = flag.Duration("timeout", 30*time.Second, "HTTP client timeout.")
)

func main() {
	flag.Parse()

	log.Printf("listen to %s in Proxy mode, timeout: %v", *configAddr, *configTimeout)
	proxy := &Proxy{
		Client: http.Client{
			Timeout: *configTimeout,
		},
	}
	if err := http.ListenAndServe(*configAddr, proxy); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal("ListenAndServe:", err)
	}
}

type Proxy struct {
	Client http.Client
}

func (p *Proxy) ServeHTTP(wr http.ResponseWriter, req *http.Request) {
	log.Println(req.RemoteAddr, " ", req.Method, " ", req.URL)

	metricMap, cerr := p.collect(req.URL)
	if cerr != nil {
		log.Println("failed to gather metrics: ", cerr)
		if errors.Is(cerr, ErrTargetInaccessible) {
			p.sendError(wr, http.StatusGatewayTimeout, cerr)
		} else {
			p.sendError(wr, http.StatusBadGateway, cerr)
		}
		return
	}

	metricNames := maps.Keys(metricMap)
	sort.Strings(metricNames)

	sb := &strings.Builder{}
	for _, name := range metricNames {
		sb.WriteString(fmt.Sprintf("%s %f\n", name, metricMap[name]))
	}

	wr.WriteHeader(http.StatusOK)
	_, werr := wr.Write([]byte(sb.String()))
	if werr != nil {
		log.Println("failed to send metrics: ", werr)
	}
}

func (p *Proxy) sendError(wr http.ResponseWriter, statusCode int, err error) {
	wr.WriteHeader(statusCode)
	_, herr := wr.Write([]byte(err.Error()))
	if herr != nil {
		log.Println("failed to send error: ", herr)
	}
}

var ErrTargetInaccessible = errors.New("inaccessible target")

func (p *Proxy) collect(target *url.URL) (map[string]float64, error) {
	resp, err := p.Client.Get(target.String())
	if err != nil {
		return nil, fmt.Errorf("%w; error scraping %q: %w", ErrTargetInaccessible, target, err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w; error reading body of %q: %w", ErrTargetInaccessible, target, err)
	}

	// Replace "\xNN" with "?" because the default parser doesn't handle them
	// well.
	re := regexp.MustCompile(`\\x..`)
	body = re.ReplaceAllFunc(body, func(s []byte) []byte {
		return []byte("?")
	})

	var vs map[string]interface{}
	err = json.Unmarshal(body, &vs)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling JSON from %q: %v", target, err)
	}

	mm := make(map[string]float64, 1000)
	for k, v := range vs {
		collectMetrics(mm, k, v)
	}
	return mm, nil
}

func collectMetrics(mm map[string]float64, k string, v interface{}) {
	name := sanitizeMetricName(k)

	switch v := v.(type) {
	case float64:
		mm[name] = v
	case bool:
		mm[name] = valToFloat(v)
	case map[string]interface{}:
		for lk, lv := range v {
			collectMetrics(mm, k+"_"+lk, lv)
		}
	case string:
		// Not supported by Prometheus.
		return
	case []interface{}:
		// Not supported by Prometheus.
		return
	default:
		fmt.Printf("Not supported unknown type: %q %#v\n", name, v)
		return
	}
}

func valToFloat(v interface{}) float64 {
	switch v := v.(type) {
	case float64:
		return v
	case bool:
		if v {
			return 1.0
		}
		return 0.0
	}
	panic(fmt.Sprintf("unexpected value type: %#v", v))
}

func sanitizeMetricName(n string) string {
	// Prometheus metric names must match the regex
	// `[a-zA-Z_:][a-zA-Z0-9_:]*`.
	// https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels
	//
	// This function replaces all non-matching ASCII characters with
	// underscores.
	//
	// In particular, it is common that expvar names contain `/` or `-`, which
	// we replace with `_` so they end up resembling more Prometheus-ideomatic
	// names.
	//
	// Non-ascii characters are not supported, and will panic as so to force
	// users to handle them explicitly.  There is no good way to handle all of
	// them automatically, as they can't be all reasonably mapped to ascii. In
	// the future, we may handle _some_ of them automatically when possible.
	// But for now, forcing the users to be explicit is the safest option, and
	// also ensures forwards compatibility.
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r
		}
		if r >= '0' && r <= '9' {
			return r
		}
		if r == '_' || r == ':' {
			return r
		}
		if r > unicode.MaxASCII {
			panic(fmt.Sprintf(
				"non-ascii character %q is unsupported, please configure the metric %q explicitly",
				r, n))
		}
		return '_'
	}, n)
}
