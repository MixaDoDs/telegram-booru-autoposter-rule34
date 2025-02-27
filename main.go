package main

import (
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"mime"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/WheatleyHDD/libgallery"
	"github.com/WheatleyHDD/libgallery/drivers/gelbooru"
	"github.com/WheatleyHDD/libgallery/drivers/rule34"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/pelletier/go-toml/v2"
)

type ConfigStruct struct {
	Service   string
	Token     string
	ChannelID int
	YourID    int
	Cooldown  int
}

var (
	Config ConfigStruct = ConfigStruct{}

	Bot            *telego.Bot
	PreloadFileID  string
	PreloadMessage *telego.Message = new(telego.Message)
	MediaType      []string
)

func main() {
	content, err := os.ReadFile("configs/config.toml") // the file is inside the local directory
	if err != nil {
		log.Panicln(err)
	}

	err = toml.Unmarshal(content, &Config)
	if err != nil {
		panic(err)
	}

	os.Mkdir("cache", 0755)

	b, err := telego.NewBot(Config.Token, telego.WithDefaultDebugLogger())
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	Bot = b

	f := prepareImage()
	err = preUpload(f)
	if err != nil {
		handleError(err)
	}

	for {
		fmt.Println(MediaType)
		sendImage()

		go func() {
			f := prepareImage()
			err = preUpload(f)
			if err != nil {
				handleError(err)
			}
		}()

		time.Sleep(time.Duration(Config.Cooldown) * time.Minute)
	}
}

func sendImage() {
	fmt.Printf("Отправка картинки в канал %v...\n", Config.ChannelID)

	content, err := os.ReadFile("configs/caption.md") // the file is inside the local directory
	if err != nil {
		log.Panicln(err)
	}

	switch MediaType[0] {
	case "video":
		v := tu.Video(
			tu.ID(int64(Config.ChannelID)),
			tu.FileFromID(PreloadFileID),
		).WithCaption(string(content)).WithParseMode("markdown")
		Bot.SendVideo(v)

		fmt.Println("Видео отправлено!")
	case "image":
		if strings.TrimSpace(MediaType[1]) == "gif" {
			v := tu.Animation(
				tu.ID(int64(Config.ChannelID)),
				tu.FileFromID(PreloadFileID),
			).WithCaption(string(content)).WithParseMode("markdown")
			Bot.SendAnimation(v)

			fmt.Println("Гиф отправлено!")
		} else {
			v := tu.Photo(
				tu.ID(int64(Config.ChannelID)),
				tu.FileFromID(PreloadFileID),
			).WithCaption(string(content)).WithParseMode("markdown")
			Bot.SendPhoto(v)

			fmt.Println("Фото отправлено!")
		}
	}

	err = Bot.DeleteMessage(tu.Delete(PreloadMessage.Chat.ChatID(), PreloadMessage.MessageID))
	if err != nil {
		log.Panicln(err)
	}

	os.RemoveAll("cache")
	os.Mkdir("cache", 0755)

	fmt.Printf("================================\n\n")
}

func preUpload(filename string) error {
	fmt.Printf("Преотправка картинки в чат %v...\n", Config.YourID)

	switch MediaType[0] {
	case "video":
		v := tu.Video(tu.ID(int64(Config.YourID)), tu.File(mustOpen(filename)))
		message, err := Bot.SendVideo(v)

		PreloadMessage = message

		if err != nil {
			return fmt.Errorf("%s", err.Error())
		}

		PreloadFileID = message.Video.FileID
	case "image":
		if strings.TrimSpace(MediaType[1]) == "gif" {
			anim := tu.Animation(tu.ID(int64(Config.YourID)), tu.File(mustOpen(filename)))
			message, err := Bot.SendAnimation(anim)

			PreloadMessage = message

			if err != nil {
				return fmt.Errorf("%s", err.Error())
			}

			PreloadFileID = message.Animation.FileID
		} else {
			p := tu.Photo(tu.ID(int64(Config.YourID)), tu.File(mustOpen(filename)))
			message, err := Bot.SendPhoto(p)

			PreloadMessage = message

			if err != nil {
				return fmt.Errorf("%s", err.Error())
			}

			PreloadFileID = message.Photo[0].FileID
		}
	}
	fmt.Printf("ID предзагруженной картинки: %v\n", PreloadFileID)

	return nil
}

func prepareImage() string {
	tags, _ := getTags()
	fmt.Println(tags)

	image, id := getNewImage(tags)
	filename, err := downloadImage(image, id)
	if err != nil {
		handleError(err)
	}
	return filename
}

func downloadImage(image libgallery.Files, id string) (string, error) {
	// Читаем часть Body, чтобы определить MIME-тип
	buffer := make([]byte, 512)
	n, err := image[0].Read(buffer)
	if err != nil && err != io.EOF {
		handleError(err)
	}

	// Определяем MIME-тип
	contentType := http.DetectContentType(buffer)
	fmt.Println(contentType)

	MediaType = strings.Split(contentType, "/")
	exts, _ := mime.ExtensionsByType(contentType)
	ext := ".bin"
	if len(exts) > 0 {
		if slices.Contains(exts, ".mp4") {
			ext = ".mp4"
		} else if slices.Contains(exts, ".webm") {
			ext = ".webm"
		} else {
			ext = exts[0] // Берём первое расширение
		}
	}

	// Создаём файл
	fileName := fmt.Sprintf("cache/%s%s", id, ext)
	file, err := os.Create(fileName)
	if err != nil {
		return "", fmt.Errorf("ошибка создания файла: %w", err)
	}
	defer file.Close()

	// Перезаписываем уже прочитанные данные
	_, err = file.Write(buffer[:n])
	if err != nil {
		return "", fmt.Errorf("ошибка записи: %w", err)
	}

	// Копируем оставшиеся данные
	_, err = io.Copy(file, image[0])
	if err != nil {
		return "", fmt.Errorf("ошибка сохранения: %w", err)
	}

	fmt.Println("Файл сохранён как", fileName)
	return fileName, nil
}

func getTags() (string, error) {
	content, err := os.ReadFile("configs/tags.txt") // the file is inside the local directory
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func getNewImage(tags string) (libgallery.Files, string) {
	page := rand.IntN(1001)
	fmt.Printf("\nSelected page: %v\n", page)

	h := driverGet()
	posts, err := h.Search(tags, uint64(page))
	if err != nil {
		handleError(err)
	}

	image := rand.IntN(len(posts))
	post := posts[image]
	fmt.Printf("Selected id: %v\n", image)
	fmt.Println(post.URL)

	src, err := h.File(post.ID)
	if err != nil {
		handleError(err)
	}

	return src, post.ID
}

func handleError(err error) {
	log.Panic(err)
}

func driverGet() libgallery.Driver {
	switch Config.Service {
	case "safebooru":
		return gelbooru.New("safebooru", "safebooru.org")
	case "rule34":
		return rule34.New()
	}
	return nil
}

func mustOpen(filename string) *os.File {
	file, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	return file
}
