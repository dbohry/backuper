package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/go-cmd/cmd"
	"github.com/robfig/cron/v3"
	"net/http"
	"strings"
)

type Config struct {
	BaseDir     string `json:"baseDir"`
	NotifyURL   string `json:"notifyURL"`
	ServiceList string `json:"serviceList"`
	Timer       string `json:"timer"`
}

func main() {
	// Read configuration from config.json
	configFile, err := os.Open("config.json")
	if err != nil {
		log.Fatalf("Error opening config file: %v", err)
	}
	defer configFile.Close()

	byteValue, _ := io.ReadAll(configFile)

	var config Config
	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		log.Fatalf("Error parsing config file: %v", err)
	}

	// Use config values
	baseDir := config.BaseDir
	notifyURL := config.NotifyURL
	serviceList := config.ServiceList
	backupDir := baseDir + "/backup"
	timer := config.Timer

	c := cron.New()
	_, _ = c.AddFunc(timer, func() {
		cleanOldBackups(backupDir)

		fmt.Println("Creating new backup...")
		services := strings.Split(serviceList, ",")
		containErrors := 0
		for _, service := range services {
			containErrors = backupService(service, baseDir)
		}

		if containErrors == 0 {
			notifyCompletion("Saturn backup completed", notifyURL)
		} else {
			notifyCompletion("Saturn backup contain errors", notifyURL)
		}
	})

	c.Start()
	select {}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func cleanOldBackups(directory string) {
	fmt.Println("Removing old backups...")
	ageLimit := 30 * 24 * time.Hour
	filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if time.Since(info.ModTime()) > ageLimit {
			if strings.HasSuffix(info.Name(), ".gz") {
				os.Remove(path)
				fmt.Println("Deleted:", path)
			}
		}
		return nil
	})
}

func backupService(serviceName string, baseDir string) int {
	var containErrors = 0
	if serviceName == "" {
		fmt.Println("Error: No service name provided.")
		return 1
	}

	fmt.Println("Stopping the service:", serviceName)
	runCommand("docker", "stop", serviceName)

	backupFileName := fmt.Sprintf(baseDir+"/backup/%s-%s.tar.gz", serviceName, time.Now().Format("2006-01-02"))
	fmt.Println("Creating backup file:", backupFileName)
	result := runCommand("tar", "-zcvf", backupFileName, fmt.Sprintf(baseDir+"/docker/%s", serviceName))
	if result.Exit != 0 {
		fmt.Println("Backup failed")
		containErrors = 1
	}

	fmt.Println("Starting the service:", serviceName)
	result = runCommand("docker", "start", serviceName)
	if result.Exit != 0 {
		fmt.Println("Failed to start service")
		containErrors = 1
	}

	if containErrors == 0 {
		fmt.Println("Backup for service", serviceName, "completed successfully.")
	} else {
		fmt.Println("Backup for service", serviceName, "completed with errors.")
	}

	return containErrors
}

func runCommand(command string, args ...string) *cmd.Status {
	commandOptions := cmd.Options{
		Buffered:  false,
		Streaming: true,
	}
	commandExec := cmd.NewCmdOptions(commandOptions, command, args...)
	statusChan := commandExec.Start()
	status := <-statusChan
	return &status
}

func notifyCompletion(message, url string) {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	resp, err := http.Post(url, "application/x-www-form-urlencoded", strings.NewReader(message))
	if err != nil {
		fmt.Println("Failed to send notification:", err)
		return
	}
	defer resp.Body.Close()
}
