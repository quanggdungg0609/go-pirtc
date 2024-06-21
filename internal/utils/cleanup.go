package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)


func deleteFiles(folderPath string) error {
	dirEntries, err := os.ReadDir(folderPath)
	if err != nil {
		return err
	}

	if len(dirEntries) == 0 {
		fmt.Printf("Folder %s is empty, no files to delete\n", folderPath)
		return nil
	}

	for _, entry := range dirEntries {
		if !entry.IsDir() {
			filePath := filepath.Join(folderPath, entry.Name())
			if err := os.Remove(filePath); err != nil {
				return err
			}
			fmt.Println("Deleted:", filePath)
		}
	}
	return nil

}

func RunPeriodicFileCleanup(folderPaths []string,hour int, stopChan <-chan struct{}){
	interval := time.Duration(hour) * time.Hour
	
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			for _, folderPath := range folderPaths {
				if err := deleteFiles(folderPath); err != nil {
					fmt.Printf("Error deleting files in %s: %v\n", folderPath, err)
				}
			}
		case <-stopChan:
			return
		}
		runtime.Gosched()
	}

}
