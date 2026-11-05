package com.systemarch.videostreaming.model;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;

public class VideoMetadata {

    public enum Status {
        PENDING,
        PROCESSING,
        DONE,
        FAILED
    }

    private String videoId;
    private String originalFilename;
    private Status status;
    private String errorMessage;
    private Double durationSeconds;
    private List<String> availableQualities;
    private Instant uploadedAt;
    private Instant processedAt;

    public VideoMetadata() {
        this.availableQualities = new ArrayList<>();
    }

    public VideoMetadata(String videoId, String originalFilename, Status status, String errorMessage,
                         Double durationSeconds, List<String> availableQualities, Instant uploadedAt, Instant processedAt) {
        this.videoId = videoId;
        this.originalFilename = originalFilename;
        this.status = status;
        this.errorMessage = errorMessage;
        this.durationSeconds = durationSeconds;
        this.availableQualities = availableQualities != null ? availableQualities : new ArrayList<>();
        this.uploadedAt = uploadedAt;
        this.processedAt = processedAt;
    }

    public static Builder builder() {
        return new Builder();
    }

    public static class Builder {
        private String videoId;
        private String originalFilename;
        private Status status;
        private String errorMessage;
        private Double durationSeconds;
        private List<String> availableQualities;
        private Instant uploadedAt;
        private Instant processedAt;

        public Builder videoId(String videoId) {
            this.videoId = videoId;
            return this;
        }

        public Builder originalFilename(String originalFilename) {
            this.originalFilename = originalFilename;
            return this;
        }

        public Builder status(Status status) {
            this.status = status;
            return this;
        }

        public Builder errorMessage(String errorMessage) {
            this.errorMessage = errorMessage;
            return this;
        }

        public Builder durationSeconds(Double durationSeconds) {
            this.durationSeconds = durationSeconds;
            return this;
        }

        public Builder availableQualities(List<String> availableQualities) {
            this.availableQualities = availableQualities;
            return this;
        }

        public Builder uploadedAt(Instant uploadedAt) {
            this.uploadedAt = uploadedAt;
            return this;
        }

        public Builder processedAt(Instant processedAt) {
            this.processedAt = processedAt;
            return this;
        }

        public VideoMetadata build() {
            return new VideoMetadata(videoId, originalFilename, status, errorMessage,
                    durationSeconds, availableQualities, uploadedAt, processedAt);
        }
    }

    public String getVideoId() {
        return videoId;
    }

    public void setVideoId(String videoId) {
        this.videoId = videoId;
    }

    public String getOriginalFilename() {
        return originalFilename;
    }

    public void setOriginalFilename(String originalFilename) {
        this.originalFilename = originalFilename;
    }

    public Status getStatus() {
        return status;
    }

    public void setStatus(Status status) {
        this.status = status;
    }

    public String getErrorMessage() {
        return errorMessage;
    }

    public void setErrorMessage(String errorMessage) {
        this.errorMessage = errorMessage;
    }

    public Double getDurationSeconds() {
        return durationSeconds;
    }

    public void setDurationSeconds(Double durationSeconds) {
        this.durationSeconds = durationSeconds;
    }

    public List<String> getAvailableQualities() {
        return availableQualities;
    }

    public void setAvailableQualities(List<String> availableQualities) {
        this.availableQualities = availableQualities;
    }

    public Instant getUploadedAt() {
        return uploadedAt;
    }

    public void setUploadedAt(Instant uploadedAt) {
        this.uploadedAt = uploadedAt;
    }

    public Instant getProcessedAt() {
        return processedAt;
    }

    public void setProcessedAt(Instant processedAt) {
        this.processedAt = processedAt;
    }
}
