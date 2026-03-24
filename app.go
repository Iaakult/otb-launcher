package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ZipVersionInfo is sourced from version.json on the server.
// Admin edits version.json to trigger a full re-download of the game zip.
type ZipVersionInfo struct {
	Version    string `json:"version"`
	Executable string `json:"executable"`
}

type LauncherConfig struct {
	Live string `json:"live"`
}

type GameProfile struct {
	ID         string
	Name       string
	RemotePath string
	InstallDir string
}

var gameProfiles = map[string]GameProfile{
	"tibia1511": {
		ID:         "tibia1511",
		Name:       "Tibia 15.11",
		RemotePath: "tibia1511",
		InstallDir: "OTBaiak Client",
	},
	"otclient": {
		ID:         "otclient",
		Name:       "OTClient",
		RemotePath: "otclient",
		InstallDir: "OTBClient",
	},
}

type App struct {
	ctx     context.Context
	logger  *logrus.Logger
	baseURL string
	appName string

	versionInfo map[string]ZipVersionInfo

	totalBytes      int64
	totalFiles      int64
	downloadedBytes int64
	downloadedFiles int64
	updateError     string
	liveURL         string

	parallel int

	activeDownloads map[string]struct{}
	mutex           sync.Mutex

	cancel chan struct{}
}

func NewApp(logger *logrus.Logger, baseURL string, appName string, parallel int) *App {
	return &App{
		logger:          logger,
		baseURL:         baseURL,
		cancel:          make(chan struct{}),
		activeDownloads: make(map[string]struct{}),
		parallel:        parallel,
		appName:         appName,
		versionInfo:     make(map[string]ZipVersionInfo),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) OpenClientLocation(gameID string) {
	fmt.Println("Opening client location")
	appDir := a.appDirectory(gameID)
	if runtime.GOOS == "darwin" {
		exec.Command("open", appDir).Start()
	} else if runtime.GOOS == "windows" {
		exec.Command("explorer", appDir).Start()
	} else if runtime.GOOS == "linux" {
		exec.Command("xdg-open", appDir).Start()
	}
}

func (a *App) Exit() {
	wailsRuntime.Quit(a.ctx)
}

func (a *App) gameBaseURL(gameID string) string {
	profile, ok := gameProfiles[gameID]
	if !ok {
		return strings.TrimRight(a.baseURL, "/") + "/"
	}
	base := strings.TrimRight(a.baseURL, "/") + "/"
	return base + strings.Trim(profile.RemotePath, "/") + "/"
}

// refreshVersion downloads the single /launcher/version.json and caches
// the entry for gameID. The file lives at baseURL, not inside the game subfolder.
func (a *App) refreshVersion(gameID string) {
	versionURL := strings.TrimRight(a.baseURL, "/") + "/version.json"
	a.logger.Infof("Fetching %s", versionURL)
	resp, err := http.Get(versionURL)
	if err != nil {
		a.logger.Errorf("Error fetching version.json: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		a.logger.Errorf("version.json returned HTTP %d", resp.StatusCode)
		return
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		a.logger.Errorf("Error reading version.json: %v", err)
		return
	}

	var cfg LauncherConfig
	if err := json.Unmarshal(body, &cfg); err == nil {
		a.liveURL = strings.TrimSpace(cfg.Live)
	}

	var all map[string]json.RawMessage
	if err := json.Unmarshal(body, &all); err != nil {
		a.logger.Errorf("Error parsing version.json: %v", err)
		return
	}
	entry, ok := all[gameID]
	if !ok {
		a.logger.Errorf("version.json has no entry for %s", gameID)
		return
	}

	var info ZipVersionInfo
	if err := json.Unmarshal(entry, &info); err != nil {
		a.logger.Errorf("Error parsing game entry %s in version.json: %v", gameID, err)
		return
	}
	if info.Version != "" {
		a.versionInfo[gameID] = info
	}
}

func (a *App) versionFilePath(gameID string) string {
	return filepath.Join(a.appDirectory(gameID), ".launcher_version")
}

func (a *App) readLocalVersion(gameID string) string {
	data, err := os.ReadFile(a.versionFilePath(gameID))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (a *App) saveLocalVersion(gameID, version string) error {
	return os.WriteFile(a.versionFilePath(gameID), []byte(version), 0644)
}

func (a *App) Version(gameID string) string {
	a.refreshVersion(gameID)
	return a.versionInfo[gameID].Version
}

func (a *App) Revision(gameID string) int {
	return 1
}

func (a *App) DownloadPercent() float64 {
	if a.totalBytes == 0 {
		// Extraction phase: report file-based progress
		if a.totalFiles > 0 {
			pct := float64(atomic.LoadInt64(&a.downloadedFiles)) / float64(a.totalFiles) * 100
			a.logger.Infof("Extracting %d/%d files (%.2f%%)", a.downloadedFiles, a.totalFiles, pct)
			return pct
		}
		return 0
	}
	percent := float64(a.downloadedBytes) / float64(a.totalBytes) * 100
	a.logger.Infof("Downloaded %d/%d files |  %d/%d bytes (%.2f%%)", a.downloadedFiles, a.totalFiles, a.downloadedBytes, a.totalBytes, percent)
	return percent
}

func (a *App) TotalFiles() int64 {
	return a.totalFiles
}

func (a *App) TotalBytes() int64 {
	return a.totalBytes
}

func (a *App) DownloadedFiles() int64 {
	return a.downloadedFiles
}

func (a *App) DownloadedBytes() int64 {
	return a.downloadedBytes
}

func (a *App) UpdateError() string {
	return a.updateError
}

func (a *App) LiveURL() string {
	if a.liveURL == "" {
		a.refreshVersion("tibia1511")
	}
	return a.liveURL
}

func (a *App) ToggleLocal(value bool) {
	a.logger.Infof("Setting enableLocal to %v", value)
	viper.Set("enableLocal", value)
	a.saveConfig()
}

func (a *App) saveConfig() {
	configPath := filepath.Join(configDirectory(a.appName), "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := viper.WriteConfigAs(configPath); err != nil {
			a.logger.Errorf("Error writing config: %v", err)
		}
		return
	}

	if err := viper.WriteConfig(); err != nil {
		a.logger.Errorf("Error writing config: %v", err)
	}
}

func (a *App) LocalEnabled() bool {
	return viper.GetBool("enableLocal")
}

func (a *App) OS() string {
	os := runtime.GOOS
	if os == "darwin" {
		return "mac"
	}
	return os
}

func (a *App) ActiveDownload() string {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	for url := range a.activeDownloads {
		return url
	}
	return ""
}

func (a *App) Update(gameID string) {
	a.updateError = ""
	a.totalFiles = 1
	a.totalBytes = 0
	a.downloadedFiles = 0
	a.downloadedBytes = 0

	// Guarantee the UI polling loop always exits, even if we return early on error.
	defer func() {
		if atomic.LoadInt64(&a.downloadedFiles) < atomic.LoadInt64(&a.totalFiles) {
			atomic.StoreInt64(&a.downloadedFiles, atomic.LoadInt64(&a.totalFiles))
		}
	}()

	a.refreshVersion(gameID)
	info := a.versionInfo[gameID]
	if info.Version == "" {
		a.updateError = "versão não encontrada para " + gameID
		a.logger.Errorf("No version info available for %s — cannot update", gameID)
		return
	}

	zipURL := strings.TrimRight(a.baseURL, "/") + "/" + gameID + ".zip"
	installDir := a.appDirectory(gameID)
	characterDataDir := filepath.Join(installDir, "characterdata")
	characterDataBackupDir := installDir + ".characterdata_backup"
	preservedCharacterData := false


	a.logger.Infof("Downloading %s zip from %s", gameID, zipURL)

	tmpFile, err := os.CreateTemp("", gameID+"-*.zip")
	if err != nil {
		a.logger.Errorf("Failed to create temp file: %v", err)
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	resp, err := http.Get(zipURL)
	if err != nil {
		a.logger.Errorf("Failed to download %s: %v", zipURL, err)
		tmpFile.Close()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		a.logger.Errorf("Failed to download %s: HTTP %d", zipURL, resp.StatusCode)
		tmpFile.Close()
		return
	}

	a.totalBytes = resp.ContentLength

	_, err = io.Copy(tmpFile, io.TeeReader(resp.Body, a))
	tmpFile.Close()
	if err != nil {
		a.updateError = "falha no download: " + err.Error()
		a.logger.Errorf("Failed to write zip: %v", err)
		return
	}

	if fileExists(characterDataDir) {
		_ = os.RemoveAll(characterDataBackupDir)
		if err := os.Rename(characterDataDir, characterDataBackupDir); err != nil {
			a.logger.Warnf("Failed to preserve characterdata: %v", err)
		} else {
			preservedCharacterData = true
			a.logger.Infof("Preserved characterdata at %s", characterDataBackupDir)
		}
	}

	a.logger.Infof("Clearing install dir %s", installDir)
	if err := os.RemoveAll(installDir); err != nil {
		a.logger.Errorf("Failed to clear install dir: %v", err)
		return
	}
	if err := os.MkdirAll(installDir, 0755); err != nil {
		a.logger.Errorf("Failed to recreate install dir: %v", err)
		return
	}

	a.logger.Infof("Extracting zip to %s", installDir)
	// Count files in zip so the UI can show extraction progress
	fileCount := countZipFiles(tmpPath)
	a.totalFiles = fileCount
	a.downloadedFiles = 0
	a.totalBytes = 0
	a.downloadedBytes = 0
	if err := unzip(tmpPath, installDir, func() {
		atomic.AddInt64(&a.downloadedFiles, 1)
	}); err != nil {
		a.updateError = "falha ao extrair: " + err.Error()
		a.logger.Errorf("Failed to extract zip: %v", err)
		return
	}

	if preservedCharacterData && fileExists(characterDataBackupDir) {
		restoredCharacterDataDir := filepath.Join(installDir, "characterdata")
		_ = os.RemoveAll(restoredCharacterDataDir)
		if err := os.Rename(characterDataBackupDir, restoredCharacterDataDir); err != nil {
			a.logger.Warnf("Failed to restore characterdata: %v", err)
		} else {
			a.logger.Infof("Restored characterdata to %s", restoredCharacterDataDir)
		}
	}

	if err := a.saveLocalVersion(gameID, info.Version); err != nil {
		a.logger.Errorf("Failed to save local version: %v", err)
		return
	}

	a.downloadedFiles = a.totalFiles
	a.logger.Infof("Update complete for %s → version %s", gameID, info.Version)
}

var mapKinds = map[int]string{
	0: "https://tibiamaps.github.io/tibia-map-data/minimap-with-markers.zip",
	1: "https://tibiamaps.github.io/tibia-map-data/minimap-without-markers.zip",
	2: "https://tibiamaps.github.io/tibia-map-data/minimap-with-grid-overlay-and-markers.zip",
	3: "https://tibiamaps.io/downloads/minimap-with-grid-overlay-without-markers",
	4: "https://tibiamaps.github.io/tibia-map-data/minimap-with-grid-overlay-and-poi-markers.zip",
}

var mapLocations = map[string]string{
	"mac":     "Contents/Resources/minimap",
	"windows": "minimap",
	"linux":   "minimap",
}

func (a *App) DownloadMaps(gameID string, kind int) {
	a.totalBytes = 0
	a.downloadedBytes = 0
	a.totalFiles = 1
	a.downloadedFiles = 0
	a.logger.Infof("Downloading %s", mapKinds[kind])
	err := a.downloadZip(mapKinds[kind], gameID, mapLocations[a.OS()], true)
	if err != nil {
		a.logger.Errorf("Error downloading %s: %v", mapKinds[kind], err)
		return
	}
}

func (a *App) NeedsUpdate(gameID string) bool {
	a.refreshVersion(gameID)
	remote := a.versionInfo[gameID].Version
	if remote == "" {
		return false
	}
	return remote != a.readLocalVersion(gameID)
}

func (a *App) appDirectory(gameID string) string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		a.logger.Errorf("Error getting config directory: %v", err)
		return ""
	}
	profile, ok := gameProfiles[gameID]
	if !ok {
		a.logger.Errorf("Unknown game profile: %s", gameID)
		return filepath.Join(configDir, a.appName)
	}

	appName := filepath.Join(a.appName, profile.InstallDir)
	if a.OS() == "mac" {
		appName = filepath.Join(a.appName, profile.InstallDir+".app")
	}
	return filepath.Join(configDir, appName)
}


func fileExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}

func readJSON(s string, d interface{}) error {
	contents, err := os.ReadFile(s)
	if err != nil {
		return err
	}
	err = json.Unmarshal(contents, &d)
	if err != nil {
		return err
	}
	return nil
}

func (a *App) downloadZip(url, gameID, dst string, progress bool) error {
	dst = filepath.Join(a.appDirectory(gameID), dst)
	err := os.MkdirAll(filepath.Dir(dst), 0755)
	if err != nil {
		return err
	}

	out, err := os.Create(filepath.Join(os.TempDir(), filepath.Base(dst)))
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed %s: HTTP %d", url, resp.StatusCode)
	}

	a.totalBytes = resp.ContentLength

	var reader io.Reader = resp.Body
	if progress {
		reader = io.TeeReader(reader, a)
	}
	_, err = io.Copy(out, reader)
	if err != nil {
		return err
	}
	out.Close()

	err = unzip(out.Name(), filepath.Dir(dst), nil)
	if err != nil {
		return err
	}

	a.downloadedFiles++

	return nil
}

func countZipFiles(src string) int64 {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return 1
	}
	defer reader.Close()
	var count int64
	for _, file := range reader.File {
		if !file.FileInfo().IsDir() {
			count++
		}
	}
	if count == 0 {
		return 1
	}
	return count
}

func unzip(src, dst string, onFile func()) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filepath.Join(dst, file.Name), 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Join(dst, filepath.Dir(file.Name)), 0755); err != nil {
			return err
		}

		rc, err := file.Open()
		if err != nil {
			return err
		}

		out, err := os.Create(filepath.Join(dst, file.Name))
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
		if onFile != nil {
			onFile()
		}
	}

	return nil
}



func (a *App) localExecutable(gameID string) string {
	name := "Contents/MacOS/client-local"
	if a.OS() == "windows" {
		name = "bin/client-local.exe"
	}
	if a.OS() == "linux" {
		name = "bin/client-local"
	}
	return filepath.Join(a.appDirectory(gameID), name)
}

var defaultExecutables = map[string]string{
	"tibia1511": "bin/client.exe",
	"otclient":  "OTBaiak OTC.exe",
}

func (a *App) executable(gameID string) string {
	exe := a.versionInfo[gameID].Executable
	if exe == "" {
		exe = defaultExecutables[gameID]
	}

	installDir := a.appDirectory(gameID)
	direct := filepath.Join(installDir, exe)
	if fileExists(direct) {
		return direct
	}

	// Support zips that extract into a single nested root directory.
	entries, err := os.ReadDir(installDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			nested := filepath.Join(installDir, entry.Name(), exe)
			if fileExists(nested) {
				return nested
			}
		}
	}

	// Last-resort fallback: find by executable filename anywhere in installDir.
	targetName := filepath.Base(exe)
	found := ""
	_ = filepath.Walk(installDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil || info.IsDir() {
			return nil
		}
		if strings.EqualFold(info.Name(), targetName) {
			found = path
			return io.EOF
		}
		return nil
	})
	if found != "" {
		return found
	}

	return direct
}

func (a *App) ensureExecutableReady(executable string) (string, error) {
	if fileExists(executable) {
		return executable, nil
	}
	return executable, fmt.Errorf("executable not found: %s", executable)
}

func (a *App) Play(gameID string, local bool) {
	// Ensure versionInfo is populated so executable() has the right name.
	if a.versionInfo[gameID].Executable == "" {
		a.refreshVersion(gameID)
	}
	executable := a.executable(gameID)
	if local {
		executable = a.localExecutable(gameID)
	}
	resolvedExecutable, err := a.ensureExecutableReady(executable)
	if err != nil {
		a.logger.Errorf("Executable not ready: %v", err)
		return
	}
	a.logger.Infof("Launching %s", resolvedExecutable)
	os.Chmod(resolvedExecutable, 0755)
	cmd := exec.Command(resolvedExecutable, "--battleeye")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		a.logger.Errorf("Failed to launch %s: %s", resolvedExecutable, err)
	}
}

func (a *App) Write(p []byte) (n int, err error) {
	n = len(p)
	atomic.AddInt64(&a.downloadedBytes, int64(n))
	return
}

func sha256Sum(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
