package main

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/Showmax/go-fqdn"
)

type lease struct {
	Ends           time.Time
	MacAddress     string
	IP             net.IP
	Hostname       string
	ClientHostname string
	ClientID       string
}

//go:embed static
var static embed.FS

//go:embed templates
var templates embed.FS

func isIPv6(ip net.IP) bool {
	return ip.To4() == nil
}

func getHostname(ip string) (string, error) {
	names, err := net.LookupAddr(ip)
	if err != nil {
		return "", err
	}
	if len(names) > 0 {
		return names[0], nil
	}
	return "", nil
}

func timeFormat(t time.Duration) string {
	if t >= time.Hour {
		hours := int(t / time.Hour)
		minutes := int((t % time.Hour) / time.Minute)
		return fmt.Sprintf("%2dh %02dm", hours, minutes)
	} else {
		minutes := int(t / time.Minute)
		seconds := int((t % time.Minute).Seconds())
		return fmt.Sprintf("%2dm %02ds", minutes, seconds)
	}
}

func parseLease(line string) (*lease, error) {
	arr := strings.Fields(line)
	if len(arr) == 2 {
		return nil, nil
	}
	if got, want := len(arr), 5; got != want {
		return nil, fmt.Errorf("illegal lease: expected %d fields, got %d", want, got)
	}

	expires, err := strconv.ParseInt(arr[0], 10, 64)
	if err != nil {
		return nil, err
	}

	hostname, _ := getHostname(arr[2])

	return &lease{
		Ends:           time.Unix(expires, 0),
		MacAddress:     arr[1],
		IP:             net.ParseIP(arr[2]),
		Hostname:       hostname,
		ClientHostname: arr[3],
		ClientID:       arr[4],
	}, nil
}

func readLeaseFile(path string) ([]lease, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// ignore
			return []lease{}, nil
		}

		return nil, err
	}

	defer f.Close()

	scanner := bufio.NewScanner(f)
	activeLeases := []lease{}

	for scanner.Scan() {
		activeLease, err := parseLease(scanner.Text())
		if err != nil {
			return nil, err
		}
		if activeLease != nil {
			activeLeases = append(activeLeases, *activeLease)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(activeLeases, func(i, j int) bool {
		return string(activeLeases[i].IP.To16()) < string(activeLeases[j].IP.To16())
	})

	return activeLeases, nil
}

func getLeases() []lease {
	results, err := readLeaseFile("/var/lib/dnsmasq/dhcp.leases")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return results
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
	leases := getLeases()

	data := struct {
		Hostname string
		Leases   []lease
		Now      string
		Total    int
	}{
		Hostname: hostname,
		Leases:   leases,
		Now:      time.Now().Format("2006-01-02 15:04:05"),
		Total:    len(leases),
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
