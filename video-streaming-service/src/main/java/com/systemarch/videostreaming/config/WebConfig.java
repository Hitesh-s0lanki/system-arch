package com.systemarch.videostreaming.config;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.scheduling.concurrent.ThreadPoolTaskExecutor;
import org.springframework.web.servlet.config.annotation.CorsRegistry;
import org.springframework.web.servlet.config.annotation.ResourceHandlerRegistry;
import org.springframework.web.servlet.config.annotation.WebMvcConfigurer;

import java.util.concurrent.Executor;

@Configuration
public class WebConfig implements WebMvcConfigurer {

    @Value("${video.upload.dir:./uploads}")
    private String uploadDir;

    @Value("${cors.allowed-origins:*}")
    private String allowedOrigins;

    /**
     * Configurable CORS support (defaults to '*' for local development/testing).
     */
    @Override
    public void addCorsMappings(CorsRegistry registry) {
        String[] origins = allowedOrigins.split(",");
        registry.addMapping("/**")
                .allowedOrigins(origins)
                .allowedMethods("GET", "POST", "PUT", "DELETE", "OPTIONS")
                .allowedHeaders("*")
                .exposedHeaders("Content-Range", "Accept-Ranges", "Content-Length", "Content-Type");
    }

    /**
     * Serve raw upload artifacts directly from the filesystem if needed.
     */
    @Override
    public void addResourceHandlers(ResourceHandlerRegistry registry) {
        String location = uploadDir.startsWith("./")
                ? "file:" + System.getProperty("user.dir") + uploadDir.substring(1) + "/"
                : "file:" + uploadDir + "/";

        registry.addResourceHandler("/uploads/**")
                .addResourceLocations(location);
    }

    /**
     * Dedicated thread pool for async FFmpeg transcoding jobs.
     */
    @Bean(name = "videoTaskExecutor")
    public Executor videoTaskExecutor() {
        ThreadPoolTaskExecutor executor = new ThreadPoolTaskExecutor();
        executor.setCorePoolSize(2);
        executor.setMaxPoolSize(4);
        executor.setQueueCapacity(50);
        executor.setThreadNamePrefix("video-async-");
        executor.initialize();
        return executor;
    }
}
