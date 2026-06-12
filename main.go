package main

import (
	"bytes"
	"context"
	"embed"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/igorkulman/bart/internal/config"
	"github.com/igorkulman/bart/internal/docker"
	"github.com/igorkulman/bart/internal/services"
	"github.com/igorkulman/bart/internal/sysinfo"
	"github.com/igorkulman/bart/internal/weather"
)

//go:embed templates/*
var templateFiles embed.FS

//go:embed static/*
var staticFiles embed.FS

var (
	cfgStore       atomic.Pointer[config.Config]
	dockerStore    atomic.Pointer[docker.Client]
	sysSampler     *sysinfo.Sampler
	weatherFetcher *weather.Fetcher
	tmpl           *template.Template
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
	SysInfo *SysInfoData
	Weather *WeatherData
	Groups  []GroupData
}

type WeatherData struct {
	Temp        string
	Description string
	FAIcon      string
}

type SysInfoData struct {
	CPUPercent  int
	MemFreeStr  string
	MemUsedPct  int
	DiskFreeStr string
	DiskUsedPct int
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
	Docker      *docker.Status
	HasLive     bool
	IntervalSec int
	// HTMXResponse is true when this tile is returned from /tile/ endpoint;
	// omits "load" from hx-trigger to avoid re-triggering immediately on outerHTML swap.
	HTMXResponse bool
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	return strings.Trim(slugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// intervalSec converts a millisecond interval to whole seconds, never returning
// less than 1 so a tiny or zero value can't render "every 0s" and hammer HTMX.
func intervalSec(ms int) int {
	if ms < 1000 {
		return 1
	}
	return ms / 1000
}

// resolvedGroup / resolvedItem carry collision-free slugs. resolveGroups is the
// single source of truth for slugs, used by both the dashboard and the per-tile
// endpoint so their slugs always agree even when names collide.
type resolvedGroup struct {
	Name  string
	Icon  string
	Slug  string
	Items []resolvedItem
}

type resolvedItem struct {
	Slug string
	Item config.Item
}

func resolveGroups(cfg *config.Config) []resolvedGroup {
	groupSeen := map[string]int{}
	groups := make([]resolvedGroup, 0, len(cfg.Services))
	for _, g := range cfg.Services {
		rg := resolvedGroup{Name: g.Name, Icon: g.Icon, Slug: uniqueSlug(slugify(g.Name), groupSeen)}
		itemSeen := map[string]int{}
		for _, item := range g.Items {
			rg.Items = append(rg.Items, resolvedItem{
				Slug: uniqueSlug(slugify(item.Name), itemSeen),
				Item: item,
			})
		}
		groups = append(groups, rg)
	}
	return groups
}

func uniqueSlug(base string, seen map[string]int) string {
	if base == "" {
		base = "item"
	}
	count := seen[base]
	seen[base] = count + 1
	if count == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, count)
}

func tileDataFor(item config.Item, groupSlug, itemSlug string) TileData {
	return TileData{
		Item:        item,
		GroupSlug:   groupSlug,
		ItemSlug:    itemSlug,
		HasLive:     item.Type != "" || item.Container != "",
		IntervalSec: intervalSec(item.UpdateIntervalMs),
		Subtitle:    template.HTML(item.Subtitle),
	}
}

// renderTemplate renders into a buffer first so a mid-render error results in a
// clean 500 instead of a half-written 200 response.
func renderTemplate(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

func formatKB(kb uint64) string {
	gb := float64(kb) / (1024 * 1024)
	if gb >= 1000 {
		return fmt.Sprintf("%d TB", int(gb/1024))
	}
	return fmt.Sprintf("%d GB", int(gb))
}

func formatBytes(b uint64) string {
	gb := float64(b) / (1024 * 1024 * 1024)
	if gb >= 1000 {
		return fmt.Sprintf("%d TB", int(gb/1024))
	}
	return fmt.Sprintf("%d GB", int(gb))
}

func pct(part, total uint64) int {
	if total == 0 {
		return 0
	}
	return int(part * 100 / total)
}

func sysinfoHandler(w http.ResponseWriter, r *http.Request) {
	if sysSampler == nil {
		http.NotFound(w, r)
		return
	}
	s := sysSampler.Current()
	data := SysInfoData{
		CPUPercent:  s.CPUPercent,
		MemFreeStr:  formatKB(s.MemAvailKB),
		MemUsedPct:  pct(s.MemTotalKB-s.MemAvailKB, s.MemTotalKB),
		DiskFreeStr: formatBytes(s.DiskFree),
		DiskUsedPct: pct(s.DiskTotal-s.DiskFree, s.DiskTotal),
	}
	renderTemplate(w, "sysinfo-inner", data)
}

func owmIconToFA(code string) string {
	switch {
	case strings.HasPrefix(code, "01"):
		if strings.HasSuffix(code, "d") {
			return "fa-solid fa-sun"
		}
		return "fa-solid fa-moon"
	case strings.HasPrefix(code, "02"):
		if strings.HasSuffix(code, "d") {
			return "fa-solid fa-cloud-sun"
		}
		return "fa-solid fa-cloud-moon"
	case strings.HasPrefix(code, "03"), strings.HasPrefix(code, "04"):
		return "fa-solid fa-cloud"
	case strings.HasPrefix(code, "09"):
		return "fa-solid fa-cloud-showers-heavy"
	case strings.HasPrefix(code, "10"):
		return "fa-solid fa-cloud-rain"
	case strings.HasPrefix(code, "11"):
		return "fa-solid fa-bolt"
	case strings.HasPrefix(code, "13"):
		return "fa-solid fa-snowflake"
	case strings.HasPrefix(code, "50"):
		return "fa-solid fa-smog"
	default:
		return "fa-solid fa-cloud"
	}
}

func weatherDataFrom(d *weather.Data) *WeatherData {
	return &WeatherData{
		Temp:        d.Temp,
		Description: d.Description,
		FAIcon:      owmIconToFA(d.IconCode),
	}
}

func weatherHandler(w http.ResponseWriter, r *http.Request) {
	if weatherFetcher == nil {
		http.NotFound(w, r)
		return
	}
	data, err := weatherFetcher.Current()
	if err != nil {
		log.Printf("weather fetch: %v", err)
		http.Error(w, "weather unavailable", http.StatusServiceUnavailable)
		return
	}
	renderTemplate(w, "weather-inner", weatherDataFrom(data))
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	cfg := getCfg()
	groups := make([]GroupData, 0, len(cfg.Services))
	for _, g := range resolveGroups(cfg) {
		gd := GroupData{Name: g.Name, Icon: g.Icon}
		for _, it := range g.Items {
			gd.Items = append(gd.Items, tileDataFor(it.Item, g.Slug, it.Slug))
		}
		groups = append(groups, gd)
	}

	pd := PageData{
		Title:   cfg.Title,
		Columns: cfg.Columns,
		Groups:  groups,
	}
	if cfg.SysInfo.Enabled && sysSampler != nil {
		s := sysSampler.Current()
		pd.SysInfo = &SysInfoData{
			CPUPercent:  s.CPUPercent,
			MemFreeStr:  formatKB(s.MemAvailKB),
			MemUsedPct:  pct(s.MemTotalKB-s.MemAvailKB, s.MemTotalKB),
			DiskFreeStr: formatBytes(s.DiskFree),
			DiskUsedPct: pct(s.DiskTotal-s.DiskFree, s.DiskTotal),
		}
	}
	if cfg.Weather.Enabled && weatherFetcher != nil {
		if data, err := weatherFetcher.Current(); err == nil {
			pd.Weather = weatherDataFrom(data)
		}
	}
	renderTemplate(w, "index.html", pd)
}

func tileHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/tile/"), "/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	groupSlug, itemSlug := parts[0], parts[1]

	var found *config.Item
	for _, g := range resolveGroups(getCfg()) {
		if g.Slug != groupSlug {
			continue
		}
		for i := range g.Items {
			if g.Items[i].Slug == itemSlug {
				found = &g.Items[i].Item
				break
			}
		}
		break
	}

	if found == nil {
		http.NotFound(w, r)
		return
	}

	badges, subtitle, err := fetchItemData(*found)
	if err != nil {
		log.Printf("fetch %q (%s): %v", found.Name, found.Type, err)
	}
	if subtitle == "" {
		subtitle = template.HTML(found.Subtitle)
	}

	var ds *docker.Status
	if dockerCli := getDocker(); found.Container != "" && dockerCli != nil {
		status := dockerCli.ContainerStatus(found.Container)
		ds = &status
	}

	renderTemplate(w, "tile", TileData{
		Item:         *found,
		GroupSlug:    groupSlug,
		ItemSlug:     itemSlug,
		Badges:       badges,
		Subtitle:     subtitle,
		Docker:       ds,
		HasLive:      true,
		IntervalSec:  intervalSec(found.UpdateIntervalMs),
		HTMXResponse: true,
	})
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

// noDirListing wraps a file handler to return 404 for directory paths, so the
// built-in FileServer doesn't expose directory listings.
func noDirListing(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		h.ServeHTTP(w, r)
	})
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
	sysSampler = sysinfo.NewSampler(cfg.SysInfo.Disk)
	sysSampler.Start()
	if cfg.Weather.Enabled {
		weatherFetcher = weather.New(weather.Config{
			APIKey:    cfg.Weather.APIKey,
			Latitude:  cfg.Weather.Latitude,
			Longitude: cfg.Weather.Longitude,
			Units:     cfg.Weather.Units,
		})
	}

	tmpl, err = template.New("").ParseFS(templateFiles, "templates/*.html")
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}

	go watchConfig(*configPath)

	mux := http.NewServeMux()
	mux.HandleFunc("/", dashboardHandler)
	mux.HandleFunc("/tile/", tileHandler)
	mux.HandleFunc("/sysinfo", sysinfoHandler)
	mux.HandleFunc("/weather", weatherHandler)
	mux.Handle("/static/", noDirListing(http.FileServer(http.FS(staticFiles))))
	mux.Handle("/assets/", noDirListing(http.StripPrefix("/assets/", http.FileServer(http.Dir(*assetsDir)))))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("bart listening on %s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
