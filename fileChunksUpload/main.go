package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
)

const maxChunkSize = 1 * 1024 * 1024 * 1024 // 1 GB in bytes

const uploadDir = "./uploads"
const combinedDir = "./combined"

func main() {
	router := gin.Default()
    router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Content-Length, X-CSRF-Token, Token, session")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}
		c.Next()
	})

	router.POST("/upload", handleUploadAndCombineChunks)
 
	router.GET("/file", handleGetFile)

	if err := router.Run(":8070"); err != nil {
		log.Fatal(err)
	}
}

func handleUploadAndCombineChunks(c *gin.Context) {
	// Get a list of chunk files in the upload directory
	chunkFiles, err := filepath.Glob(filepath.Join(uploadDir, "*"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list chunk files"})
		return
	}

	if len(chunkFiles) > 0 {
		// If chunk files exist, combine them
		handleCombineChunks(c)
	} else {
		// Otherwise, handle file upload
		handleUploadChunk(c)
		// After uploading, initiate chunk combining
		handleCombineChunks(c)
	}
}

func handleUploadChunk(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File upload failed"})
		return
	}
	defer file.Close()

	originalFilename := header.Filename

	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
		return
	}

	chunkPath := filepath.Join(uploadDir, originalFilename)
	out, err := os.Create(chunkPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create chunk file"})
		return
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save chunk data"})
		return
	}

	//c.JSON(http.StatusOK, gin.H{"message": "Chunk uploaded successfully"})
	c.JSON(http.StatusOK, gin.H{"message": "File uploaded successfully!", "filename": originalFilename})
}

func handleCombineChunks(c *gin.Context) {
	chunkFiles, err := filepath.Glob(filepath.Join(uploadDir, "*"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list chunk files"})
		return
	}

	sort.Slice(chunkFiles, func(i, j int) bool {
		return extractIndex(chunkFiles[i]) < extractIndex(chunkFiles[j])
	})

	var originalFilename string
	if len(chunkFiles) > 0 {
		originalFilename = filepath.Base(chunkFiles[0])
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No chunks found"})
		return
	}

	if err := os.MkdirAll(combinedDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create combined directory"})
		return
	}

	combinedPath := filepath.Join(combinedDir, originalFilename)
	combinedFile, err := os.Create(combinedPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create combined file"})
		return
	}
	defer combinedFile.Close()

	for _, chunkFile := range chunkFiles {
		chunkData, err := os.ReadFile(chunkFile)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read chunk data"})
			return
		}
		if _, err := combinedFile.Write(chunkData); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write combined data"})
			return
		}
	}

	if err := deleteChunkFiles(chunkFiles); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete chunk files"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Chunks combined and saved successfully"})
}

func deleteChunkFiles(files []string) error {
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			return err
		}
	}
	return nil
}

func extractIndex(filename string) int {
	base := filepath.Base(filename)
	indexStr := base[len("chunk_"):]
	index, _ := strconv.Atoi(indexStr)
	return index
}


func handleGetFile(c *gin.Context) {
	filename := c.Query("filename")
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Filename is required"})
		return
	}

	filePath := filepath.Join(combinedDir, filename)

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		}
		return
	}
	defer file.Close()

	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "application/octet-stream")
	c.File(filePath)
}