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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/ulikunitz/xz/lzma"
)

type File struct {
	LocalFile    string `json:"localfile"`
	PackedHash   string `json:"packedhash"`
	PackedSize   int    `json:"packedsize"`
	URL          string `json:"url"`
	UnpackedHash string `json:"unpackedhash"`
	UnpackedSize int    `json:"unpackedsize"`
}

type AssetsInfo struct {
	Files   []File `json:"files"`
	Version int    `json:"version"`
}

type ClientInfo struct {
	Revision   int    `json:"revision"`
	Version    string `json:"version"`
	Files      []File `json:"files"`
	Executable string `json:"executable"`
	Generation string `json:"generation"`
	Variant    string `json:"variant"`
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

	clientInfo map[string]ClientInfo
	assetsInfo map[string]AssetsInfo

	totalBytes      int64
	totalFiles      int64
	downloadedBytes int64
	downloadedFiles int64

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
		clientInfo:      make(map[string]ClientInfo),
		assetsInfo:      make(map[string]AssetsInfo),
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
	os.Exit(0)
}

func (a *App) remoteClientJSON(gameID string) string {
	return "client." + a.OS() + ".json"
}

func (a *App) remoteAssetsJSON(gameID string) string {
	return "assets." + a.OS() + ".json"
}

func (a *App) gameBaseURL(gameID string) string {
	profile, ok := gameProfiles[gameID]
	if !ok {
		return strings.TrimRight(a.baseURL, "/") + "/"
	}
	base := strings.TrimRight(a.baseURL, "/") + "/"
	return base + strings.Trim(profile.RemotePath, "/") + "/"
}

func (a *App) resolveDownloadURL(gameID, remotePath string) string {
	if strings.HasPrefix(remotePath, "http://") || strings.HasPrefix(remotePath, "https://") {
		parsed, err := url.Parse(remotePath)
		if err != nil {
			return remotePath
		}
		parsed.Path = escapeURLPathSegments(parsed.Path)
		return parsed.String()
	}
	return a.gameBaseURL(gameID) + escapeURLPathSegments(strings.TrimLeft(remotePath, "/"))
}

func escapeURLPathSegments(rawPath string) string {
	parts := strings.Split(rawPath, "/")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func (a *App) refreshManifests(gameID string) {
	err := a.downloadFile(a.resolveDownloadURL(gameID, a.remoteClientJSON(gameID)), gameID, "client.json", false)
	if err != nil {
		a.logger.Errorf("Error downloading %s for %s: %v", a.remoteClientJSON(gameID), gameID, err)
		return
	}

	var clientInfo ClientInfo
	err = readJSON(filepath.Join(a.appDirectory(gameID), "client.json"), &clientInfo)
	if err != nil {
		a.logger.Errorf("Error reading %s for %s: %v", "client.json", gameID, err)
		return
	}
	a.clientInfo[gameID] = clientInfo

	err = a.downloadFile(a.resolveDownloadURL(gameID, a.remoteAssetsJSON(gameID)), gameID, "assets.json", false)
	if err != nil {
		a.logger.Errorf("Error downloading %s for %s: %v", a.remoteAssetsJSON(gameID), gameID, err)
		return
	}

	var assetsInfo AssetsInfo
	err = readJSON(filepath.Join(a.appDirectory(gameID), "assets.json"), &assetsInfo)
	if err != nil {
		a.logger.Errorf("Error reading %s for %s: %v", "assets.json", gameID, err)
		return
	}
	a.assetsInfo[gameID] = assetsInfo
}

func (a *App) Version(gameID string) string {
	a.refreshManifests(gameID)
	return a.clientInfo[gameID].Version
}

func (a *App) Revision(gameID string) int {
	a.refreshManifests(gameID)
	return a.clientInfo[gameID].Revision
}

func (a *App) DownloadPercent() float64 {
	if a.totalBytes == 0 {
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
	files, err := a.filesToUpdate(gameID)
	if err != nil {
		a.logger.Errorf("Error checking for updates for %s: %v", gameID, err)
		return
	}

	a.totalFiles = int64(len(files))
	a.totalBytes = 0
	a.downloadedFiles = 0
	a.downloadedBytes = 0
	for _, file := range files {
		a.totalBytes += int64(file.PackedSize)
	}

	if len(files) == 0 {
		return
	}

	workers := a.parallel
	if workers > len(files) {
		workers = len(files)
	}

	queue := make(chan File, len(files))

	wg := sync.WaitGroup{}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-a.cancel:
					return
				case <-a.ctx.Done():
					return
				case file, ok := <-queue:
					if !ok {
						return
					}
					a.mutex.Lock()
					a.activeDownloads[file.URL] = struct{}{}
					a.mutex.Unlock()
					err := a.downloadFile(a.resolveDownloadURL(gameID, file.URL), gameID, file.LocalFile, true)
					a.mutex.Lock()
					delete(a.activeDownloads, file.URL)
					a.mutex.Unlock()
					if err != nil {
						a.logger.Errorf("Error downloading %s: %v", file.URL, err)
						return
					}
					a.logger.Debugf("Downloaded %s", file.URL)
				}
			}
		}()
	}

	for _, file := range files {
		queue <- file
	}
	close(queue)
	wg.Wait()
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
	a.refreshManifests(gameID)
	files, err := a.filesToUpdate(gameID)
	if err != nil {
		a.logger.Errorf("Error checking for updates for %s: %v", gameID, err)
		return false
	}
	return len(files) > 0
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

func (a *App) filesToUpdate(gameID string) ([]File, error) {
	var files []File
	assets := a.assetsInfo[gameID]
	client := a.clientInfo[gameID]
	filesTocheck := append(assets.Files, client.Files...)

	mutex := sync.Mutex{}
	wg := sync.WaitGroup{}
	wg.Add(len(filesTocheck))

	for _, file := range filesTocheck {
		go func(file File) {
			defer wg.Done()

			localFilePath := filepath.Join(a.appDirectory(gameID), file.LocalFile)
			if !fileExists(localFilePath) {
				a.logger.Infof("File %s does not exist", localFilePath)
				mutex.Lock()
				files = append(files, file)
				mutex.Unlock()
			} else {
				localHash, err := sha256Sum(localFilePath)
				if err != nil {
					a.logger.Errorf("Error reading local file: %s\n", err)
					return
				}

				if localHash != file.UnpackedHash {
					a.logger.Infof("File %s has changed (local: %s, remote: %s)", localFilePath, string(localHash), file.UnpackedHash)
					mutex.Lock()
					files = append(files, file)
					mutex.Unlock()
				}
			}
		}(file)
	}

	wg.Wait()

	return files, nil
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

	err = unzip(out.Name(), filepath.Dir(dst))
	if err != nil {
		return err
	}

	a.downloadedFiles++

	return nil
}

func unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			err := os.MkdirAll(filepath.Join(dst, f.Name), 0755)
			if err != nil {
				return err
			}
			continue
		}

		err := os.MkdirAll(filepath.Join(dst, filepath.Dir(f.Name)), 0755)
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.Create(filepath.Join(dst, f.Name))
		if err != nil {
			return err
		}

		_, err = io.Copy(out, rc)
		if err != nil {
			return err
		}

		out.Close()
		rc.Close()
	}

	return nil
}

func (a *App) downloadFile(url, gameID, dst string, progress bool) error {
	a.logger.Infof("Downloading %s to %s", url, dst)
	dst = filepath.Join(a.appDirectory(gameID), dst)
	err := os.MkdirAll(filepath.Dir(dst), 0755)
	if err != nil {
		return err
	}

	out, err := os.Create(dst)
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

	var reader io.Reader = resp.Body
	if progress {
		reader = io.TeeReader(reader, a)
	}

	if filepath.Ext(dst) != ".lzma" && filepath.Ext(url) == ".lzma" {
		lzmaReader, err := lzma.NewReader(reader)
		if err != nil {
			return err
		}
		reader = lzmaReader
	}

	_, err = io.Copy(out, reader)
	if err != nil {
		return err
	}

	atomic.AddInt64(&a.downloadedFiles, 1)

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

func (a *App) executable(gameID string) string {
	return filepath.Join(a.appDirectory(gameID), a.clientInfo[gameID].Executable)
}

func (a *App) Play(gameID string, local bool) {
	executable := a.executable(gameID)
	if local {
		executable = a.localExecutable(gameID)
	}
	a.logger.Infof("Launching %s", executable)
	os.Chmod(a.executable(gameID), 0755)
	if err := syscall.Exec(executable, []string{"--battleeye"}, os.Environ()); err != nil {
		a.logger.Errorf("Failed to launch %s: %s | attempting regular fork", executable, err)
		cmd := exec.Command(executable, "--battleeye")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()
		if err := cmd.Start(); err != nil {
			a.logger.Errorf("Failed to launch %s: %s", executable, err)
		}
		os.Exit(0)
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
