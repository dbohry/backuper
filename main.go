package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-cmd/cmd"
	"net/http"
	"strings"
)

func main() {
	fmt.Println("Removing old backups...")

	// Define the backup directory and age limit
	backupDir := getEnv("BACKUP_DIR", "./backup")
	notifyURL := getEnv("NOTIFY_URL", "https://notify.lhamacorp.com/backup")
	serviceList := getEnv("BACKUP_SERVICES", "")
	ageLimit := 30 * 24 * time.Hour // 30 days

	// Clean old backups
	cleanOldBackups(backupDir, ageLimit)

	fmt.Println("Creating new backup...")
	services := strings.Split(serviceList, ",")
	containErrors := 0
	for _, service := range services {
		containErrors = backupService(service)
	}

	// Notify completion
	if containErrors == 0 {
		notifyCompletion("Saturn backup completed", notifyURL)
	} else {
		notifyCompletion("Saturn backup contain errors", notifyURL)
	}

}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func cleanOldBackups(directory string, ageLimit time.Duration) {
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

func backupService(serviceName string) int {
	var containErrors = 0
	if serviceName == "" {
		fmt.Println("Error: No service name provided.")
		return 1
	}

	fmt.Println("Stopping the service:", serviceName)
	runCommand("docker", "stop", serviceName)

	backupFileName := fmt.Sprintf("backup/%s-%s.tar.gz", serviceName, time.Now().Format("2006-01-02"))
	fmt.Println("Creating backup file:", backupFileName)
	result := runCommand("tar", "-zcvf", backupFileName, fmt.Sprintf("docker/%s", serviceName))
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
	resp, err := http.Post(url, "application/x-www-form-urlencoded", strings.NewReader(message))
	if err != nil {
		fmt.Println("Failed to send notification:", err)
		return
	}
	defer resp.Body.Close()
}
