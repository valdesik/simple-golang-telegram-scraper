package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chromedp/chromedp"
	"github.com/joho/godotenv"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Post represents a single post from the Telegram channel.
type Post struct {
	Text string `json:"text"`
}

// Config holds the application configuration.
type Config struct {
	ChannelName    string
	ScrollDuration time.Duration
	OutputDir      string
}

// NewConfig creates a new Config instance from environment variables and user input.
func NewConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		return nil, fmt.Errorf("error loading .env file: %w", err)
	}

	channelName := getEnvOrPrompt("CHANNEL_NAME", "Enter the channel name: ", extractChannelName)
	scrollDuration, err := readScrollDuration()
	if err != nil {
		return nil, fmt.Errorf("error reading scroll duration: %w", err)
	}

	outputDir := getEnvOrDefault("OUTPUT_DIR", "posts")

	return &Config{
		ChannelName:    channelName,
		ScrollDuration: scrollDuration,
		OutputDir:      outputDir,
	}, nil
}

// getEnvOrPrompt retrieves an environment variable or prompts the user if not found.
func getEnvOrPrompt(envVar, prompt string, transformFunc func(string) string) string {
	value := os.Getenv(envVar)
	if value == "" {
		fmt.Print(prompt)
		_, _ = fmt.Scanln(&value)
		value = transformFunc(value)
	}
	return value
}

// getEnvOrDefault retrieves an environment variable or returns a default value.
func getEnvOrDefault(envVar, defaultValue string) string {
	value := os.Getenv(envVar)
	if value == "" {
		return defaultValue
	}
	return value
}

// readScrollDuration prompts the user for the scrolling duration.
func readScrollDuration() (time.Duration, error) {
	var minutes int
	for {
		fmt.Print("How many minutes do you want to scroll? (1-60): ")
		_, err := fmt.Scanf("%d", &minutes)
		if err == nil && minutes >= 1 && minutes <= 60 {
			break
		}
		fmt.Println("Invalid input. Please enter a number between 1 and 60.")
	}
	return time.Duration(minutes) * time.Minute, nil
}

// extractChannelName extracts the channel name from the given string.
func extractChannelName(channelString string) string {
	channelString = strings.TrimPrefix(channelString, "@")
	channelString = strings.TrimPrefix(channelString, "https://t.me/")
	parts := strings.SplitN(channelString, "/", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return parts[0]
}

// createUniqueFilename generates a unique filename for the collected posts.
func createUniqueFilename(channelName string) string {
	timestamp := time.Now().Format("20060102150405")
	return fmt.Sprintf("%s_posts_%s.json", channelName, timestamp)
}

// createFolderIfNotExists creates the output directory if it doesn't exist.
func createFolderIfNotExists(folderName string) error {
	if _, err := os.Stat(folderName); os.IsNotExist(err) {
		if err := os.Mkdir(folderName, 0755); err != nil {
			return fmt.Errorf("error creating output directory: %w", err)
		}
		log.Printf("Folder '%s' created.\n", folderName)
	} else {
		log.Printf("Folder '%s' already exists.\n", folderName)
	}
	return nil
}

// scrapeChannel scrapes posts from the specified Telegram channel.
func scrapeChannel(ctx context.Context, url string, cfg *Config) error {
	log.Println("Navigating to the channel...")
	if err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(".tgme_widget_message_text", chromedp.ByQuery),
	); err != nil {
		return fmt.Errorf("error navigating to channel: %w", err)
	}
	log.Println("Successfully navigated to the channel.")

	postSet := make(map[string]bool)
	filename := createUniqueFilename(cfg.ChannelName)
	filePath := filepath.Join(cfg.OutputDir, filename)

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("error opening/creating JSON file: %w", err)
	}
	defer file.Close()

	log.Println("Starting to scroll and collect unique posts...")
	endTime := time.Now().Add(cfg.ScrollDuration)
	for time.Now().Before(endTime) {
		if err := collectPosts(ctx, postSet, file); err != nil {
			log.Println(err)
		}

		remainingTime := time.Until(endTime)
		log.Printf("Collected %d unique posts so far. Time left: %02d:%02d",
			len(postSet), int(remainingTime.Minutes()), int(remainingTime.Seconds())%60)
	}

	log.Println("Finished scrolling and collecting unique posts.")
	return nil
}

// collectPosts handles the scraping and storing of posts.
func collectPosts(ctx context.Context, postSet map[string]bool, file *os.File) error {
	var contentList []string
	if err := chromedp.Run(ctx,
		chromedp.ScrollIntoView(".tgme_widget_message_text"),
		chromedp.Sleep(4*time.Second),
		chromedp.Evaluate(`Array.from(document.querySelectorAll('.tgme_widget_message_text')).map(el => el.innerText)`, &contentList),
	); err != nil {
		return fmt.Errorf("error scraping posts: %w", err)
	}

	for _, content := range contentList {
		if err := processPost(content, postSet, file); err != nil {
			log.Println(err)
		}
	}
	return nil
}

// processPost processes and saves a single post if it is unique.
func processPost(content string, postSet map[string]bool, file *os.File) error {
	if !postSet[content] {
		postSet[content] = true
		post := Post{Text: content}
		if err := writePostToFile(post, file); err != nil {
			return err
		}
		log.Println("Collected unique post:", content)
	}
	return nil
}

// writePostToFile writes a post to the specified file.
func writePostToFile(post Post, file *os.File) error {
	postData, err := json.Marshal(post)
	if err != nil {
		return fmt.Errorf("error marshalling post: %w", err)
	}

	if _, err := file.Write(postData); err != nil {
		return fmt.Errorf("error writing post to file: %w", err)
	}
	if _, err := file.WriteString("\n"); err != nil {
		return fmt.Errorf("error writing newline to file: %w", err)
	}
	return nil
}

func main() {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("start-maximized", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancel()

	cfg, err := NewConfig()
	if err != nil {
		log.Fatalf("error creating configuration: %v", err)
	}

	url := "https://t.me/s/" + cfg.ChannelName

	if err := createFolderIfNotExists(cfg.OutputDir); err != nil {
		log.Fatalf("error creating output directory: %v", err)
	}

	if err := scrapeChannel(ctx, url, cfg); err != nil {
		log.Fatalf("error scraping channel: %v", err)
	}
}
