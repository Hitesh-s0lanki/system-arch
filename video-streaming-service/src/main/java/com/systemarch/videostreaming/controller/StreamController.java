package com.systemarch.videostreaming.controller;

import com.systemarch.videostreaming.model.VideoMetadata;
import com.systemarch.videostreaming.service.VideoProcessingService;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.*;
import org.springframework.web.bind.annotation.*;
import org.springframework.web.multipart.MultipartFile;

import java.io.IOException;
import java.io.RandomAccessFile;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.List;
import java.util.UUID;

@RestController
@RequestMapping("/api/stream")
public class StreamController {

    private static final Logger log = LoggerFactory.getLogger(StreamController.class);

    private final VideoProcessingService processingService;

    @Value("${video.upload.dir:./uploads}")
    private String uploadDir;

    public StreamController(VideoProcessingService processingService) {
        this.processingService = processingService;
    }

    // -------------------------------------------------------------------------
    // 1. Upload
    // -------------------------------------------------------------------------

    @PostMapping(value = "/upload", consumes = MediaType.MULTIPART_FORM_DATA_VALUE)
    public ResponseEntity<VideoMetadata> upload(@RequestParam("file") MultipartFile file) throws IOException {
        if (file.isEmpty()) {
            return ResponseEntity.badRequest().build();
        }

        String videoId = UUID.randomUUID().toString();
        Path videoDir  = resolveVideoDir(videoId);
        Files.createDirectories(videoDir);

        Path inputFile = videoDir.resolve("original.mp4");
        file.transferTo(inputFile);
        log.info("Uploaded '{}' as videoId={} ({})", file.getOriginalFilename(), videoId, humanReadableSize(file.getSize()));

        VideoMetadata meta = processingService.initializeVideo(videoId, file.getOriginalFilename(), inputFile);
        return ResponseEntity.accepted().body(meta);
    }

    // -------------------------------------------------------------------------
    // 2. Master playlist
    // -------------------------------------------------------------------------

    @GetMapping("/{videoId}/master.m3u8")
    public ResponseEntity<byte[]> masterPlaylist(@PathVariable String videoId) throws IOException {
        Path playlist = resolveVideoDir(videoId).resolve("master.m3u8");
        return servePlaylist(playlist);
    }

    // -------------------------------------------------------------------------
    // 3. Rendition playlist + TS segments
    // -------------------------------------------------------------------------

    @GetMapping("/{videoId}/{quality}/{filename}")
    public ResponseEntity<byte[]> serveSegment(
            @PathVariable String videoId,
            @PathVariable String quality,
            @PathVariable String filename,
            @RequestHeader(value = HttpHeaders.RANGE, required = false) String rangeHeader
    ) throws IOException {

        Path filePath = resolveVideoDir(videoId).resolve(quality).resolve(filename);

        if (!Files.exists(filePath)) {
            return ResponseEntity.notFound().build();
        }

        if (filename.endsWith(".m3u8")) {
            return servePlaylist(filePath);
        }

        if (filename.endsWith(".ts")) {
            return serveSegment(filePath, rangeHeader);
        }

        return ResponseEntity.status(HttpStatus.UNSUPPORTED_MEDIA_TYPE).build();
    }

    // -------------------------------------------------------------------------
    // 4. List videos
    // -------------------------------------------------------------------------

    @GetMapping("/videos")
    public ResponseEntity<List<VideoMetadata>> listVideos() {
        return ResponseEntity.ok(processingService.getAllMetadata());
    }

    // -------------------------------------------------------------------------
    // 5. Video info
    // -------------------------------------------------------------------------

    @GetMapping("/{videoId}/info")
    public ResponseEntity<VideoMetadata> videoInfo(@PathVariable String videoId) {
        return processingService.getMetadata(videoId)
                .map(ResponseEntity::ok)
                .orElse(ResponseEntity.notFound().build());
    }

    // -------------------------------------------------------------------------
    // Internal helpers
    // -------------------------------------------------------------------------

    private ResponseEntity<byte[]> servePlaylist(Path path) throws IOException {
        if (!Files.exists(path)) {
            return ResponseEntity.notFound().build();
        }
        byte[] content = Files.readAllBytes(path);
        HttpHeaders headers = new HttpHeaders();
        headers.setContentType(MediaType.parseMediaType("application/vnd.apple.mpegurl"));
        headers.setCacheControl(CacheControl.noCache());
        return ResponseEntity.ok().headers(headers).body(content);
    }

    private ResponseEntity<byte[]> serveSegment(Path path, String rangeHeader) throws IOException {
        long fileSize = Files.size(path);
        HttpHeaders headers = new HttpHeaders();
        headers.setContentType(MediaType.parseMediaType("video/mp2t"));
        headers.set(HttpHeaders.ACCEPT_RANGES, "bytes");

        if (rangeHeader == null || rangeHeader.isBlank()) {
            headers.setContentLength(fileSize);
            return ResponseEntity.ok().headers(headers).body(Files.readAllBytes(path));
        }

        String[] parts = rangeHeader.replace("bytes=", "").split("-");
        long start = Long.parseLong(parts[0].trim());
        long end   = (parts.length > 1 && !parts[1].isBlank())
                ? Long.parseLong(parts[1].trim())
                : fileSize - 1;

        if (start > end || start >= fileSize) {
            headers.set(HttpHeaders.CONTENT_RANGE, "bytes */" + fileSize);
            return ResponseEntity.status(HttpStatus.REQUESTED_RANGE_NOT_SATISFIABLE)
                    .headers(headers).build();
        }

        end = Math.min(end, fileSize - 1);
        long length = end - start + 1;

        byte[] data = readRange(path, start, length);

        headers.set(HttpHeaders.CONTENT_RANGE, "bytes " + start + "-" + end + "/" + fileSize);
        headers.setContentLength(length);

        return ResponseEntity.status(HttpStatus.PARTIAL_CONTENT).headers(headers).body(data);
    }

    private byte[] readRange(Path path, long offset, long length) throws IOException {
        try (RandomAccessFile raf = new RandomAccessFile(path.toFile(), "r")) {
            raf.seek(offset);
            byte[] buf = new byte[(int) length];
            raf.readFully(buf);
            return buf;
        }
    }

    private Path resolveVideoDir(String videoId) {
        String base = uploadDir.startsWith("./")
                ? System.getProperty("user.dir") + uploadDir.substring(1)
                : uploadDir;
        return Paths.get(base, videoId);
    }

    private String humanReadableSize(long bytes) {
        if (bytes < 1024) return bytes + " B";
        if (bytes < 1024 * 1024) return String.format("%.1f KB", bytes / 1024.0);
        if (bytes < 1024 * 1024 * 1024) return String.format("%.1f MB", bytes / (1024.0 * 1024));
        return String.format("%.1f GB", bytes / (1024.0 * 1024 * 1024));
    }
}
