package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sort"
	"text/template"
	"time"

	"github.com/Showmax/go-fqdn"
	leases "github.com/npotts/go-dhcpd-leases"
)

const leasesFile = "/var/lib/dhcp/dhcpd.leases"

//go:embed static
var static embed.FS

//go:embed templates
var templates embed.FS

// filterLeases removes expired leases and find the latest lease for each IP
// return a sorted list of leases
func filterLeases(inputs []leases.Lease) []leases.Lease {
	now := time.Now()
	var result []leases.Lease
	for _, l := range inputs {
		// iif lease is expired, don't return it
		if l.Ends.Before(now) {
			continue
		}

		// search for an existing lease with the same IP
		var found bool
		for i, r := range result {
			if l.IP.Equal(r.IP) {
				found = true
				if l.Ends.After(r.Ends) {
					result[i] = l
				}
				break
			}
		}
		if !found {
			result = append(result, l)
		}
	}
	// sorting IP on string value isn't always a good idea
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].IP.String() < result[j].IP.String()
	})
	return result
}

func getLeases() []leases.Lease {
	f, err := os.Open(leasesFile)

	if err != nil {
		fmt.Println(err)
	}
	r := leases.Parse(f)
	f.Close()
	result := filterLeases(r)
	return result
}

func handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.Method != "GET" {
		http.NotFound(w, r)
		return
	}

	tmpl, err := template.ParseFS(templates, "templates/index.html")
	if err != nil {
		log.Println(err)
	}

	hostname, _ := fqdn.FqdnHostname()

	data := struct {
		Hostname string
		Leases   []leases.Lease
		Now      string
	}{
		Hostname: hostname,
		Leases:   getLeases(),
		Now:      time.Now().Format("2006-01-02 15:04:05"),
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Println(err)
	}
}

func main() {
	port := ":7777"
	log.Printf("Listenning %s\n", port)

	// handlers
	staticFS, _ := fs.Sub(static, "static")
	static_handler := http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))

	http.Handle("/static/", static_handler)
	http.HandleFunc("/", handler)

	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
