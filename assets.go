package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getFileExtension(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return "bin"
	}
	return parts[1]
}

func getAssetPath(mediaType string) string {
	randIDString := make([]byte, 32)
	_, err := rand.Read(randIDString)
	if err != nil {
		panic("failed to generate random bytes")
	}

	id := base64.RawURLEncoding.EncodeToString(randIDString)
	ext := getFileExtension(mediaType)
	return fmt.Sprintf("%s.%s", id, ext)
}

func (cfg apiConfig) getObjectURL(key string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, key)
}

func (cfg apiConfig) getAssetDiskPath(assetPath string) string {
	return filepath.Join(cfg.assetsRoot, assetPath)
}

func (cfg apiConfig) getAssetURL(assetPath string) string {
	return fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetPath)
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		filePath,
	)
    outBuf := bytes.Buffer{}
	cmd.Stdout = &outBuf

	if err := cmd.Run(); err != nil {
		log.Printf("Got an error when running the command: %v\n", err)
		return "", fmt.Errorf("ffprobe error: %v", err)
	}

	var data struct {
		Streams []struct {
			Index 		int 	`json:"index"`
			AspectRatio string 	`json:"display_aspect_ratio"`
			Width		int		`json:"width"`
			Height		int		`json:"height"`
		} `json:"streams"`
	}

	if err := json.Unmarshal(outBuf.Bytes(), &data); err != nil {
		log.Fatalf("Error unmarshalling JSON: %v", err)
		return "", fmt.Errorf("Could not parse ffprobe output: %v", err)
	}

	if len(data.Streams) == 0 {
		return "", errors.New("no video streams found")
	}

	ar := data.Streams[0].AspectRatio
	width := data.Streams[0].Width
	height := data.Streams[0].Height

	if ar != "" && ar == "16:9" {
		return ar, nil
	}
	if ar != "" && ar == "9:16" {
		return ar, nil
	}
	if ar == "" {
		if width == 16*height/9 {
			return "16:9", nil
		}
		if height == 16*width/9 {
			return "9:16", nil
		}
		return "other", nil
	}
	return "other", nil
}
