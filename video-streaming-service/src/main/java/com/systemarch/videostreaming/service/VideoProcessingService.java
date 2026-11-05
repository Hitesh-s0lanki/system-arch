package com.systemarch.videostreaming.service;

import com.systemarch.videostreaming.model.VideoMetadata;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.scheduling.annotation.Async;
import org.springframework.stereotype.Service;

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.time.Instant;
import java.util.*;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ConcurrentHashMap;

@Service
public class VideoProcessingService {

    private static final Logger log = LoggerFactory.getLogger(VideoProcessingService.class);

    // -----------------------------------------------------------------------
    // Quality profile definitions
    // -----------------------------------------------------------------------

    private static final List<QualityProfile> QUALITY_PROFILES = List.of(
        new QualityProfile("360p",  640,  360, "800k",   "856k",   "1200k"),
        new QualityProfile("720p",  1280, 720, "2800k",  "3000k",  "6000k"),
        new QualityProfile("1080p", 1920, 1080,"5000k",  "5350k",  "7500k")
    );

    @Value("${video.upload.dir:./uploads}")
    private String uploadDir;

    @Value("${ffmpeg.path:ffmpeg}")
    private String ffmpegPath;

    @Value("${hls.segment.time:4}")
    private int hlsSegmentTime;

    /** In-memory store: videoId → metadata. */
    private final Map<String, VideoMetadata> metadataStore = new ConcurrentHashMap<>();

    // -----------------------------------------------------------------------
    // Public API
    // -----------------------------------------------------------------------

    /**
     * Register a newly uploaded video and kick off async transcoding.
     */
    public VideoMetadata initializeVideo(String videoId, String originalFilename, Path inputFile) {
        VideoMetadata meta = VideoMetadata.builder()
                .videoId(videoId)
                .originalFilename(originalFilename)
                .status(VideoMetadata.Status.PENDING)
                .uploadedAt(Instant.now())
                .availableQualities(new ArrayList<>())
                .build();
        metadataStore.put(videoId, meta);
        processVideoAsync(videoId, inputFile);
        return meta;
    }

    /** Return metadata for a single video, or empty if not found. */
    public Optional<VideoMetadata> getMetadata(String videoId) {
        return Optional.ofNullable(metadataStore.get(videoId));
    }

    /** Return metadata for all known videos. */
    public List<VideoMetadata> getAllMetadata() {
        return new ArrayList<>(metadataStore.values());
    }

    // -----------------------------------------------------------------------
    // Async transcoding pipeline
    // -----------------------------------------------------------------------

    @Async("videoTaskExecutor")
    public CompletableFuture<Void> processVideoAsync(String videoId, Path inputFile) {
        VideoMetadata meta = metadataStore.get(videoId);
        meta.setStatus(VideoMetadata.Status.PROCESSING);
        log.info("[{}] Starting HLS transcoding for '{}'", videoId, inputFile.getFileName());

        try {
            Path outputDir = resolveOutputDir(videoId);
            Files.createDirectories(outputDir);

            List<String> completedQualities = new ArrayList<>();

            for (QualityProfile profile : QUALITY_PROFILES) {
                Path qualityDir = outputDir.resolve(profile.name());
                Files.createDirectories(qualityDir);

                log.info("[{}] Transcoding → {} ({}x{})", videoId, profile.name(), profile.width(), profile.height());
                runFfmpeg(buildFfmpegCommand(inputFile, qualityDir, profile), videoId);
                completedQualities.add(profile.name());
                log.info("[{}] Finished {}", videoId, profile.name());
            }

            // Probe duration from the first quality's playlist (or fall back to 0)
            double duration = probeDuration(outputDir.resolve(completedQualities.get(0))
                    .resolve(completedQualities.get(0) + ".m3u8"));

            // Write the master playlist that ties all renditions together
            writeMasterPlaylist(outputDir, completedQualities);

            meta.setStatus(VideoMetadata.Status.DONE);
            meta.setAvailableQualities(completedQualities);
            meta.setDurationSeconds(duration);
            meta.setProcessedAt(Instant.now());
            log.info("[{}] Transcoding complete. Duration: {}s, Qualities: {}", videoId, duration, completedQualities);

        } catch (Exception e) {
            log.error("[{}] Transcoding failed: {}", videoId, e.getMessage(), e);
            meta.setStatus(VideoMetadata.Status.FAILED);
            meta.setErrorMessage(e.getMessage());
            meta.setProcessedAt(Instant.now());
        }

        return CompletableFuture.completedFuture(null);
    }

    // -----------------------------------------------------------------------
    // FFmpeg helpers
    // -----------------------------------------------------------------------

    private List<String> buildFfmpegCommand(Path input, Path qualityDir, QualityProfile profile) {
        String segmentPattern = qualityDir.resolve(profile.name() + "_%03d.ts").toString();
        String playlistPath   = qualityDir.resolve(profile.name() + ".m3u8").toString();

        return List.of(
            ffmpegPath, "-y",
            "-i", input.toString(),
            "-vf", "scale=w=" + profile.width() + ":h=" + profile.height() + ":force_original_aspect_ratio=decrease",
            "-c:a", "aac", "-ar", "48000", "-b:a", "128k",
            "-c:v", "h264", "-profile:v", "main",
            "-crf", "20",
            "-sc_threshold", "0",
            "-g", "48", "-keyint_min", "48",
            "-hls_time", String.valueOf(hlsSegmentTime),
            "-hls_playlist_type", "vod",
            "-b:v", profile.bitrate(),
            "-maxrate", profile.maxrate(),
            "-bufsize", profile.bufsize(),
            "-hls_segment_filename", segmentPattern,
            playlistPath
        );
    }

    private void runFfmpeg(List<String> command, String videoId) throws IOException, InterruptedException {
        log.debug("[{}] CMD: {}", videoId, String.join(" ", command));
        ProcessBuilder pb = new ProcessBuilder(command);
        pb.redirectErrorStream(true);
        Process process = pb.start();

        try (BufferedReader reader = new BufferedReader(new InputStreamReader(process.getInputStream()))) {
            String line;
            while ((line = reader.readLine()) != null) {
                log.debug("[{}] ffmpeg: {}", videoId, line);
            }
        }

        int exitCode = process.waitFor();
        if (exitCode != 0) {
            throw new RuntimeException("FFmpeg exited with code " + exitCode + " for command: " + command.get(0));
        }
    }

    // -----------------------------------------------------------------------
    // Master playlist
    // -----------------------------------------------------------------------

    private void writeMasterPlaylist(Path outputDir, List<String> qualities) throws IOException {
        StringBuilder sb = new StringBuilder();
        sb.append("#EXTM3U\n");
        sb.append("#EXT-X-VERSION:3\n\n");

        for (String quality : qualities) {
            QualityProfile profile = QUALITY_PROFILES.stream()
                    .filter(p -> p.name().equals(quality))
                    .findFirst()
                    .orElseThrow();

            long bandwidthBps = parseBitrateToBps(profile.bitrate());
            sb.append(String.format("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d,CODECS=\"avc1.4d001f,mp4a.40.2\",NAME=\"%s\"\n",
                    bandwidthBps, profile.width(), profile.height(), quality));
            sb.append(quality).append("/").append(quality).append(".m3u8\n\n");
        }

        Path masterPlaylist = outputDir.resolve("master.m3u8");
        Files.writeString(masterPlaylist, sb.toString());
        log.info("Master playlist written → {}", masterPlaylist);
    }

    private double probeDuration(Path playlistPath) {
        if (!Files.exists(playlistPath)) return 0.0;
        try {
            return Files.lines(playlistPath)
                    .filter(l -> l.startsWith("#EXTINF:"))
                    .mapToDouble(l -> {
                        String val = l.substring("#EXTINF:".length()).replace(",", "").trim();
                        try { return Double.parseDouble(val); } catch (NumberFormatException e) { return 0.0; }
                    })
                    .sum();
        } catch (IOException e) {
            log.warn("Could not probe duration from {}: {}", playlistPath, e.getMessage());
            return 0.0;
        }
    }

    private Path resolveOutputDir(String videoId) {
        String base = uploadDir.startsWith("./")
                ? System.getProperty("user.dir") + uploadDir.substring(1)
                : uploadDir;
        return Paths.get(base, videoId);
    }

    private long parseBitrateToBps(String bitrate) {
        if (bitrate.endsWith("k")) {
            return Long.parseLong(bitrate.substring(0, bitrate.length() - 1)) * 1000;
        }
        return Long.parseLong(bitrate);
    }

    private record QualityProfile(
            String name,
            int width,
            int height,
            String bitrate,
            String maxrate,
            String bufsize
    ) {}
}
