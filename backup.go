package main

import (
	"encoding/binary"
	"log"
	"os"
	"time"
)

func backupWorker() {
	// Every 5 seconds, back up the current state of the canvas to a binary file
	for {
		func() {
			time.Sleep(5 * time.Second)

			// Open the backup file
			file, err := os.Create("placesh.chunks")
			if err != nil {
				log.Printf("Error creating backup file: %v", err)
				return
			}
			defer file.Close()

			// Write the chunks to the file
			for _, chunk := range chunks {
				if err := binary.Write(file, binary.LittleEndian, chunk); err != nil {
					log.Printf("Error writing chunk to backup: %v", err)
					continue
				}
			}
		}()

		log.Println("Backup completed successfully")
	}
}

func restoreBackup() {
	file, err := os.Open("placesh.chunks")
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No backup file found — skipping restore")
		} else {
			log.Printf("Error opening backup file: %v", err)
		}
		return
	}
	defer file.Close()

	var restored []*Chunk
	for {
		var c Chunk
		err := binary.Read(file, binary.LittleEndian, &c)
		if err != nil {
			if err.Error() == "EOF" {
				break // Finished reading
			}
			log.Printf("Error reading chunk from backup: %v", err)
			return
		}
		restored = append(restored, &c)
	}

	// Replace the current chunks with the restored ones
	chunks = restored
	log.Println("Backup restored successfully")
}
