package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	// Upload limit 1GB
	uploadLimit := 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, int64(uploadLimit))
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not find video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized to access that video", nil)
		return
	}

	vFile, vHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Could not read form file", err)
		return
	}
	defer vFile.Close()

	mediaType, _, err := mime.ParseMediaType(vHeader.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", nil)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, fmt.Sprintf("Unsupported mediaType: %s", mediaType), nil)
		return
	}

	tmpFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create temp file", err)
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, vFile)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not copy file to disk", err)
		return
	}
	_, err = tmpFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not rewind to start of temp file", err)
		return
	}

	//Check aspect ratio of video
	aspectRatio, err := getVideoAspectRatio(tmpFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not determine aspect ratio of video file", err)
		return
	}
	viewingMode := ""
	switch aspectRatio {
	case "16:9":
		viewingMode = "landscape"
	case "9:16":
		viewingMode = "portrait"
	default:
		viewingMode = "other"
	}

	key := fmt.Sprintf("%s/%s", viewingMode, getAssetPath(mediaType))
	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:			aws.String(cfg.s3Bucket),
		Key:			aws.String(key),
		Body:			tmpFile,
		ContentType:	aws.String(mediaType),
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not store file", err)
		return
	}

	newURL := cfg.getObjectURL(key)

	video.UpdatedAt = time.Now()
	video.VideoURL = &newURL

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not update video", err)
		return
	}
	respondWithJSON(w, http.StatusOK, video)
}
