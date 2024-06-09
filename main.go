package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

// pdftotext line feed as a separator for pages
const pageSeparator rune = '\f'

type image struct {
	Data     string `json:"data"`
}

type page struct {
	PageNumber int     `json:"pageNumber"`
	Text       string  `json:"text"`
	Images     []image `json:"images"`
}

type response struct {
	Pages []page `json:"pages"`
}

func encodePNGDataToHTMLData(data []byte) string {
	encodedData := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:image/png;base64,%s", encodedData)
}

func runPDFToText(inputFile string, startPage int, endPage int) (string, error) {
	args := []string{
		"-raw",
	}

	if startPage > 0 {
		args = append(args, "-f", fmt.Sprintf("%d", startPage))
	}

	if endPage > 0 {
		args = append(args, "-l", fmt.Sprintf("%d", endPage))
	}

	args = append(args, inputFile, "-")

	cmd := exec.Command("pdftotext", args...)
	var out bytes.Buffer

	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// runs pdfimages on inputFile and extract images to outputDir
func runPDFImages(inputFile string, startPage int, endPage int, outputDir string) error {
	args := []string{
		"-png",
		"-p",
	}

	if startPage > 0 {
		args = append(args, "-f", fmt.Sprintf("%d", startPage))
	}

	if endPage > 0 {
		args = append(args, "-l", fmt.Sprintf("%d", endPage))
	}

	args = append(args, inputFile, outputDir)

	cmd := exec.Command("pdfimages", args...)
	err := cmd.Run()
	if err != nil {
		return nil
	}
	return nil
}

func loadImages(imageDir string) (map[int][]string, error) {
	files, err := os.ReadDir(imageDir)
	if err != nil {
		return nil, err
	}

	results := map[int][]string{}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		tokens := strings.Split(file.Name(), "-")
		nTokens := len(tokens)
		pageToken := tokens[nTokens - 2]
		pageNumber, err := strconv.Atoi(pageToken)
		if err != nil {
			return nil, err
		}

		fullFileName := path.Join(imageDir, file.Name())
		imageData, err := os.ReadFile(fullFileName)
		if err != nil {
			return nil, err
		}

		existingImages, ok := results[pageNumber]
		if !ok {
			existingImages = []string{}
		}
		existingImages = append(existingImages, encodePNGDataToHTMLData(imageData))
		results[pageNumber] = existingImages
	}

	return results, nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			tempDir, err := os.MkdirTemp("", "pdfworkdir-")
			if err != nil {
				log.Printf("ERROR os.MkDirTemp failed: %s", err.Error())

				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]any{
					"error": err.Error(),
				})
				return
			}
			defer os.RemoveAll(tempDir)

			if err := r.ParseMultipartForm(1024 * 1024 * 128); err != nil {
				log.Printf("ERROR r.ParseMultipartForm failed: %s", err.Error())

				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]any{
					"error": err.Error(),
				})
				return
			}

			file, _, err := r.FormFile("file")
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]any{
					"error": err.Error(),
				})
				return
			}
			defer file.Close()

			tempFileName := tempDir + "/input.pdf"
			tempFile, err := os.Create(tempFileName)
			if err != nil {
				log.Printf("ERROR os.Create failed: %s", err.Error())

				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]any{
					"error": err.Error(),
				})
				return
			}
			defer tempFile.Close()

			_, err = io.Copy(tempFile, file)
			if err != nil {
				log.Printf("ERROR io.Copy failed: %s", err.Error())

				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]any{
					"error": err.Error(),
				})
				return
			}

			startPage := 1
			endPage := 0

			if r.URL.Query().Get("startPage") != "" {
				parsed, err := strconv.Atoi(r.URL.Query().Get("startPage"))
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]any{
						"error": "invalid startPage argument",
					})
					return
				}
				startPage = parsed
				if startPage < 1 {
					startPage = 1
				}
			}

			if r.URL.Query().Get("endPage") != "" {
				parsed, err := strconv.Atoi(r.URL.Query().Get("endPage"))
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]any{
						"error": "invalid endPage argument",
					})
					return
				}
				endPage = parsed
				if endPage < 0 {
					endPage = 0
				}
			}

			pdfText, err := runPDFToText(tempFileName, startPage, endPage)
			if err != nil {
				log.Printf("ERROR runPDFToText failed: %s", err.Error())

				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]any{
					"error": err.Error(),
				})
				return
			}

			resp := response{}
			pageTexts := strings.Split(pdfText, string(pageSeparator))

			for i, t := range pageTexts {
				resp.Pages = append(resp.Pages, page{
					PageNumber: startPage + i,
					Text:       t,
					Images:     []image{},
				})
			}

			if r.URL.Query().Get("images") == "1" {
				imageDir := tempDir + "/images/"

				if err := os.Mkdir(imageDir, 0755); err != nil {
					log.Printf("ERROR os.Mkdir failed: %s", err.Error())

					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]any{
						"error": err.Error(),
					})
					return
				}

				if err := runPDFImages(tempFileName, startPage, endPage, imageDir); err != nil {
					log.Printf("ERROR runPDFImages failed: %s", err.Error())

					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]any{
						"error": err.Error(),
					})
					return
				}

				imageData, err := loadImages(imageDir);
				if err != nil {
					log.Printf("ERROR loadImages failed: %s", err.Error())

					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]any{
						"error": err.Error(),
					})
					return
				}

				for i := 0; i < len(resp.Pages); i++ {
					pageImages, ok := imageData[resp.Pages[i].PageNumber]
					if ok {
						for _, imageData := range pageImages {
							resp.Pages[i].Images = append(resp.Pages[i].Images, image{
								Data: imageData,
							})
						}
					}
				}
			}

			json.NewEncoder(w).Encode(resp)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	listenAddress := os.Getenv("LISTEN_ADDRESS")
	if listenAddress == "" {
		listenAddress = "127.0.0.1:9001"
	}

	if err := http.ListenAndServe(listenAddress, nil); err != nil {
		panic(err)
	}
}
