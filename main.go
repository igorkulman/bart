package main

import (
	"embed"
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/igorkulman/bart/internal/config"
	"github.com/igorkulman/bart/internal/docker"
	"github.com/igorkulman/bart/internal/services"
)

//go:embed templates/*
var templateFiles embed.FS

//go:embed static/*
var staticFiles embed.FS

var (
	cfgStore    atomic.Pointer[config.Config]
	dockerStore atomic.Pointer[docker.Client]
	tmpl        *template.Template
)

// getCfg returns the currently loaded configuration. It is swapped atomically
// by watchConfig, so callers always get a consistent snapshot.
func getCfg() *config.Config { return cfgStore.Load() }

// getDocker returns the current docker client (recreated if the socket changes).
func getDocker() *docker.Client { return dockerStore.Load() }

// watchConfig polls the config file and reloads it on change so edits take
// effect without restarting the container. A failed reload keeps the previous
// config so a malformed edit can't take the dashboard down.
func watchConfig(path string) {
	var last time.Time
	if fi, err := os.Stat(path); err == nil {
		last = fi.ModTime()
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		if !fi.ModTime().After(last) {
			continue
		}
		last = fi.ModTime()

		newCfg, err := config.Load(path)
		if err != nil {
			log.Printf("config reload failed, keeping previous version: %v", err)
			continue
		}
		if newCfg.DockerSocket != getCfg().DockerSocket {
			dockerStore.Store(docker.NewClient(newCfg.DockerSocket))
		}
		cfgStore.Store(newCfg)
		log.Printf("config reloaded from %s", path)
	}
}

type PageData struct {
	Title   string
	Columns int
	Groups  []GroupData
}

type GroupData struct {
	Name  string
	Icon  string
	Items []TileData
}

type TileData struct {
	config.Item
	GroupSlug   string
	ItemSlug    string
	Badges      []services.Badge
	Subtitle    template.HTML
	Docker      *docker.DockerStatus
	HasLive     bool
	IntervalSec int
	// HTMXResponse is true when this tile is returned from /tile/ endpoint;
	// omits "load" from hx-trigger to avoid re-triggering immediately on outerHTML swap.
	HTMXResponse bool
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	return slugRe.ReplaceAllString(strings.ToLower(s), "-")
}

func intervalSec(ms int) int {
	return ms / 1000
}

func tileDataFor(item config.Item, groupSlug string) TileData {
	return TileData{
		Item:        item,
		GroupSlug:   groupSlug,
		ItemSlug:    slugify(item.Name),
		HasLive:     item.Type != "" || item.Container != "",
		IntervalSec: intervalSec(item.UpdateIntervalMs),
		Subtitle:    template.HTML(item.Subtitle),
	}
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	cfg := getCfg()
	var groups []GroupData
	for _, g := range cfg.Services {
		gd := GroupData{Name: g.Name, Icon: g.Icon}
		gslug := slugify(g.Name)
		for _, item := range g.Items {
			gd.Items = append(gd.Items, tileDataFor(item, gslug))
		}
		groups = append(groups, gd)
	}

	data := PageData{
		Title:   cfg.Title,
		Columns: cfg.Columns,
		Groups:  groups,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func tileHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/tile/"), "/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	groupSlug, itemSlug := parts[0], parts[1]

	cfg := getCfg()
	var foundItem *config.Item
	for _, g := range cfg.Services {
		if slugify(g.Name) == groupSlug {
			for i := range g.Items {
				if slugify(g.Items[i].Name) == itemSlug {
					foundItem = &g.Items[i]
					break
				}
			}
		}
		if foundItem != nil {
			break
		}
	}

	if foundItem == nil {
		http.NotFound(w, r)
		return
	}

	badges, subtitle, _ := fetchItemData(*foundItem)
	if subtitle == "" {
		subtitle = template.HTML(foundItem.Subtitle)
	}

	var ds *docker.DockerStatus
	if dockerCli := getDocker(); foundItem.Container != "" && dockerCli != nil {
		status := dockerCli.ContainerStatus(foundItem.Container)
		ds = &status
	}

	data := TileData{
		Item:         *foundItem,
		GroupSlug:    groupSlug,
		ItemSlug:     itemSlug,
		Badges:       badges,
		Subtitle:     subtitle,
		Docker:       ds,
		HasLive:      true,
		IntervalSec:  intervalSec(foundItem.UpdateIntervalMs),
		HTMXResponse: true,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "tile", data); err != nil {
		log.Printf("tile template error: %v", err)
	}
}

func fetchItemData(item config.Item) ([]services.Badge, template.HTML, error) {
	switch strings.ToLower(item.Type) {
	case "jellyfin":
		return services.FetchJellyfin(item)
	case "miniflux":
		return services.FetchMiniflux(item)
	case "sonarr", "radarr", "prowlarr":
		return services.FetchArr(item)
	case "transmission":
		return services.FetchTransmission(item)
	case "uptimekuma":
		return services.FetchUptimeKuma(item)
	case "homeassistant":
		return services.FetchHomeAssistant(item)
	case "unifi":
		return services.FetchUnifi(item)
	default:
		return nil, "", nil
	}
}

func main() {
	configPath := flag.String("config", "config.yml", "path to config file")
	addr := flag.String("addr", ":8080", "listen address")
	assetsDir := flag.String("assets", "./assets", "directory to serve at /assets/")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	cfgStore.Store(cfg)
	dockerStore.Store(docker.NewClient(cfg.DockerSocket))

	tmpl, err = template.New("").ParseFS(templateFiles, "templates/*.html")
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}

	go watchConfig(*configPath)

	http.HandleFunc("/", dashboardHandler)
	http.HandleFunc("/tile/", tileHandler)
	http.Handle("/static/", http.FileServer(http.FS(staticFiles)))
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(*assetsDir))))

	log.Printf("bart listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
