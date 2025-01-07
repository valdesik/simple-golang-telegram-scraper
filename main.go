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

type Post struct {
	Text string `json:"text"`
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
	envErr := godotenv.Load()
	if envErr != nil {
		log.Fatal("Error loading .env file")
	}
	channelName := os.Getenv("CHANNEL_NAME")
	if channelName == "" {
		channelName = readChannelName()
	}

	url := "https://t.me/s/" + channelName

	log.Println("Navigating to the channel...")
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(".tgme_widget_message_text", chromedp.ByQuery),
	)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Successfully navigated to the channel.")
	var timeToScroll int
	timeToScroll = readNumber("How many minutes do you want to scroll? (1-60): ")
	scrollDuration := time.Duration(timeToScroll) * time.Minute
	endTime := time.Now().Add(scrollDuration)

	postSet := make(map[string]bool)
	filename := createUniqueFilename(channelName)
	postsDir := "posts"
	err = createFolderIfNotExists(postsDir)
	if err != nil {
		log.Fatal(err)
	}
	filePath := filepath.Join(postsDir, filename)
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal("Error opening/creating JSON file:", err)
	}
	defer file.Close()

	log.Println("Starting to scroll and collect unique posts...")
	for time.Now().Before(endTime) {
		var contentList []string

		err := chromedp.Run(ctx,
			chromedp.ScrollIntoView(".tgme_widget_message_text"),
			chromedp.Sleep(2*time.Second),
			chromedp.Evaluate(`Array.from(document.querySelectorAll('.tgme_widget_message_text')).map(el => el.innerText)`, &contentList),
		)
		if err != nil {
			log.Fatal(err)
		}

		for _, content := range contentList {
			if !postSet[content] {
				postSet[content] = true
				post := Post{Text: content}
				postData, err := json.Marshal(post)
				if err != nil {
					log.Println("Error marshalling post:", err)
					continue
				}

				_, err = file.Write(postData)
				if err != nil {
					log.Println("Error writing post to file:", err)
					continue
				}
				_, err = file.WriteString("\n")
				if err != nil {
					log.Println("Error writing newline to file:", err)
					continue
				}
			}
		}

		remainingTime := time.Until(endTime)
		log.Printf("Collected %d unique posts so far. Time left: %02d:%02d",
			len(postSet), int(remainingTime.Minutes()), int(remainingTime.Seconds())%60)
	}

	log.Println("Finished scrolling and collecting unique posts.")
}

func readNumber(prompt string) int {
	var number int
	for {
		fmt.Print(prompt)
		_, err := fmt.Scanf("%d", &number)
		if err == nil && number >= 1 && number <= 60 {
			break
		}
		fmt.Println("Invalid input. Please enter a number between 1 and 60.")
	}
	return number

}

func readChannelName() string {
	var channelName string
	fmt.Print("Enter the channel name: ")
	_, err := fmt.Scanln(&channelName)
	if err != nil {
		log.Fatal("Error reading channel name:", err)
	}
	return extractChannelName(channelName)
}

func extractChannelName(channelString string) string {
	channelName := strings.TrimPrefix(channelString, "@")
	channelName = strings.TrimPrefix(channelName, "https://t.me/")

	parts := strings.Split(channelName, "/")
	return parts[len(parts)-1]
}

func createUniqueFilename(channelName string) string {
	// Отримуємо поточний час у форматі, зручному для назви файлу
	timestamp := time.Now().Format("20060102150405") // Рік, місяць, день, година, хвилина, секунда
	return fmt.Sprintf("%s_posts_%s.json", channelName, timestamp)
}

func createFolderIfNotExists(folderName string) error {
	if _, err := os.Stat(folderName); os.IsNotExist(err) {
		// Створюємо папку, якщо вона не існує
		err := os.Mkdir(folderName, 0755) // 0755 встановлює права доступу
		if err != nil {
			return err
		}
		fmt.Printf("Folder '%s' created.\n", folderName)
	} else {
		fmt.Printf("Folder '%s' already exists.\n", folderName)
	}

	return nil
}
