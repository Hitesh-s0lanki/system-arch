# 🎬 video-streaming-service

A minimal **Java Spring Boot** service that demonstrates **Netflix-style HLS (HTTP Live Streaming)** adaptive bitrate video streaming. Upload an MP4, the service transcodes it into multiple quality renditions using FFmpeg, and any HLS-compatible player can stream it with automatic quality switching.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         video-streaming-service                             │
│                                                                             │
│  ┌──────────┐   POST /upload    ┌────────────────────────────────────────┐  │
│  │          │ ───────────────► │         VideoProcessingService          │  │
│  │  Client  │                  │                                        │  │
│  │ (curl /  │                  │  ┌─────────┐  FFmpeg  ┌─────────────┐  │  │
│  │  player) │                  │  │  360p   │ ───────► │ 360p/*.ts   │  │  │
│  │          │                  │  │  720p   │ ───────► │ 720p/*.ts   │  │  │
│  │          │                  │  │  1080p  │ ───────► │ 1080p/*.ts  │  │  │
│  │          │                  │  └─────────┘          └──────┬──────┘  │  │
│  │          │                  │                              │          │  │
│  │          │                  │                     master.m3u8        │  │
│  │          │                  └──────────────────────────────┼─────────┘  │
│  │          │                                                 │            │
│  │          │   GET master.m3u8  ◄────────────────────────────┘            │
│  │          │                                                              │
│  │  ABR     │   GET 720p.m3u8  ◄── StreamController (/api/stream/...)      │
│  │  Switch  │   GET 720p_001.ts                                            │
│  │   ▼  ▲   │   GET 360p_002.ts  (Range: bytes=0-131071)                   │
│  └──────────┘                                                              │
└─────────────────────────────────────────────────────────────────────────────┘

Upload flow:
  MP4 ──► save original.mp4 ──► FFmpeg (async, per quality) ──► .ts segments
                                                              ──► quality.m3u8
                                                              ──► master.m3u8

Playback flow:
  Player fetches master.m3u8
    └─► picks best quality rendition playlist
          └─► fetches .ts chunks in order
                └─► switches quality based on bandwidth estimate
```

---

## How HLS Works (the Netflix way)

### 1. Segmentation
FFmpeg slices the video into small **Transport Stream (`.ts`) chunks**, typically 4–10 seconds each. Clients download these sequentially, buffering a few ahead.

### 2. Rendition Playlists (`.m3u8`)
Each quality level has its own playlist listing the segment filenames and their durations:
```
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:4
#EXTINF:4.000000,
720p_000.ts
#EXTINF:4.000000,
720p_001.ts
...
#EXT-X-ENDLIST
```

### 3. Master Playlist
One top-level `master.m3u8` lists all renditions with metadata so the player can pick:
```
#EXTM3U
#EXT-X-VERSION:3

#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360,CODECS="avc1.4d001f,mp4a.40.2",NAME="360p"
360p/360p.m3u8

#EXT-X-STREAM-INF:BANDWIDTH=2800000,RESOLUTION=1280x720,CODECS="avc1.4d001f,mp4a.40.2",NAME="720p"
720p/720p.m3u8

#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080,CODECS="avc1.4d001f,mp4a.40.2",NAME="1080p"
1080p/1080p.m3u8
```

### 4. Adaptive Bitrate Switching (ABR)
The player monitors download speed. If the current segment takes too long, it switches to a lower-quality rendition on the next chunk boundary — seamlessly. This is exactly what Netflix, YouTube, and Twitch do.

### 5. HTTP Range Requests
`.ts` chunks are served with `Accept-Ranges: bytes`. Safari and AVFoundation always issue `Range` requests; the controller handles HTTP 206 Partial Content correctly.

---

## Prerequisites

| Tool     | Version | Purpose                      |
|----------|---------|------------------------------|
| Java     | 17+     | Runtime                      |
| Maven    | 3.6+    | Build                        |
| FFmpeg   | 4.x+    | Video transcoding            |

### Install FFmpeg

**macOS (Homebrew)**
```bash
brew install ffmpeg
```

**Ubuntu / Debian**
```bash
sudo apt update && sudo apt install -y ffmpeg
```

**Windows**
Download from [https://ffmpeg.org/download.html](https://ffmpeg.org/download.html) and add `ffmpeg.exe` to your `PATH`.

Verify: `ffmpeg -version`

---

## Configuration & Environment Variables

The service supports zero-config defaults, but can be configured using a `.env` file or environment variables.

Copy the template:
```bash
cp .env.example .env
```

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Port for the HTTP API |
| `VIDEO_UPLOAD_DIR` | `./uploads` | Directory for raw uploads and `.ts` / `.m3u8` chunks |
| `MAX_FILE_SIZE` | `500MB` | Max upload size per file |
| `MAX_REQUEST_SIZE` | `500MB` | Max multipart request size |
| `FFMPEG_PATH` | `ffmpeg` | Path to FFmpeg binary (e.g. `/opt/homebrew/bin/ffmpeg` or `ffmpeg`) |
| `HLS_TIME` | `4` | Segment chunk duration in seconds |
| `CORS_ALLOWED_ORIGINS` | `*` | Allowed CORS origins (e.g. `*` or `http://localhost:3000`) |

---

## Run

```bash
cd video-streaming-service
mvn spring-boot:run
```

The service starts on **http://localhost:8080**.

Uploaded files and HLS output land in `./uploads/` relative to the working directory.

---

## API Reference & curl Examples

### Upload a video
```bash
curl -X POST http://localhost:8080/api/stream/upload \
  -F "file=@/path/to/your/video.mp4"
```
Response (HTTP 202):
```json
{
  "videoId": "550e8400-e29b-41d4-a716-446655440000",
  "originalFilename": "video.mp4",
  "status": "PENDING",
  "uploadedAt": "2024-01-15T10:30:00Z"
}
```
Transcoding runs in the background. Poll `/info` to track progress.

---

### Check processing status
```bash
curl http://localhost:8080/api/stream/{videoId}/info
```
Response when done:
```json
{
  "videoId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "DONE",
  "durationSeconds": 142.5,
  "availableQualities": ["360p", "720p", "1080p"],
  "processedAt": "2024-01-15T10:33:12Z"
}
```

---

### Get the master playlist
```bash
curl http://localhost:8080/api/stream/{videoId}/master.m3u8
```

---

### Stream in a player
Open any HLS-compatible player and point it at:
```
http://localhost:8080/api/stream/{videoId}/master.m3u8
```

**VLC**
```bash
vlc http://localhost:8080/api/stream/{videoId}/master.m3u8
```

**ffplay**
```bash
ffplay http://localhost:8080/api/stream/{videoId}/master.m3u8
```

**Browser (hls.js)**
```html
<script src="https://cdn.jsdelivr.net/npm/hls.js@latest"></script>
<video id="video" controls></video>
<script>
  const video = document.getElementById('video');
  const src = 'http://localhost:8080/api/stream/{videoId}/master.m3u8';
  if (Hls.isSupported()) {
    const hls = new Hls();
    hls.loadSource(src);
    hls.attachMedia(video);
  } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
    video.src = src; // Safari native HLS
  }
</script>
```

---

### List all videos
```bash
curl http://localhost:8080/api/stream/videos
```

---

### Fetch a specific segment (with Range)
```bash
curl -H "Range: bytes=0-131071" \
  http://localhost:8080/api/stream/{videoId}/720p/720p_000.ts \
  --output chunk.ts
```

---

## Quality Profiles

| Quality | Resolution  | Video Bitrate | Audio  |
|---------|------------|---------------|--------|
| 360p    | 640×360    | 800 kbps      | 128 kbps AAC |
| 720p    | 1280×720   | 2800 kbps     | 128 kbps AAC |
| 1080p   | 1920×1080  | 5000 kbps     | 128 kbps AAC |

Keyframe interval is locked to 48 frames so segment boundaries are identical across renditions — a requirement for seamless ABR switching.

---

## Project Structure

```
video-streaming-service/
├── pom.xml
├── README.md
└── src/main/
    ├── java/com/systemarch/videostreaming/
    │   ├── VideoStreamingApplication.java   # @SpringBootApplication + @EnableAsync
    │   ├── config/
    │   │   └── WebConfig.java               # CORS, static resources, async executor
    │   ├── controller/
    │   │   └── StreamController.java        # REST endpoints, Range header handling
    │   ├── service/
    │   │   └── VideoProcessingService.java  # FFmpeg orchestration, in-memory state
    │   └── model/
    │       └── VideoMetadata.java           # Upload/processing state DTO
    └── resources/
        └── application.properties
```

---

## Notes

- **No database** — metadata lives in a `ConcurrentHashMap`. Restart the service and it forgets all videos (the files on disk remain).
- **No auth** — CORS is fully open. This is intentional for a local demo.
- **FFmpeg must be on `PATH`** — the service calls it via `ProcessBuilder`.
- For production: add persistent storage (PostgreSQL), a job queue (RabbitMQ / SQS), CDN delivery (CloudFront / Fastly), and DRM.
