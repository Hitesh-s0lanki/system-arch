package com.systemarch.videostreaming;

import io.github.cdimascio.dotenv.Dotenv;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.scheduling.annotation.EnableAsync;

import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;

@SpringBootApplication
@EnableAsync
public class VideoStreamingApplication {

    private static final Logger log = LoggerFactory.getLogger(VideoStreamingApplication.class);

    public static void main(String[] args) {
        loadDotenv();
        SpringApplication.run(VideoStreamingApplication.class, args);
    }

    /**
     * Loads .env file if present (checking current directory and subfolder),
     * and exports entries into system properties for Spring Boot.
     */
    private static void loadDotenv() {
        Path envInCurrentDir = Paths.get(".env");
        Path envInServiceDir = Paths.get("video-streaming-service", ".env");

        Dotenv dotenv = null;
        if (Files.exists(envInCurrentDir)) {
            dotenv = Dotenv.configure().ignoreIfMissing().load();
            log.info("Loaded environment variables from {}", envInCurrentDir.toAbsolutePath());
        } else if (Files.exists(envInServiceDir)) {
            dotenv = Dotenv.configure().directory("video-streaming-service").ignoreIfMissing().load();
            log.info("Loaded environment variables from {}", envInServiceDir.toAbsolutePath());
        } else {
            dotenv = Dotenv.configure().ignoreIfMissing().load();
        }

        dotenv.entries().forEach(entry -> {
            if (System.getProperty(entry.getKey()) == null && System.getenv(entry.getKey()) == null) {
                System.setProperty(entry.getKey(), entry.getValue());
            }
        });
    }
}
