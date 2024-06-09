package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"log"
)

// pdftotext line feed as a separator for pages
const pageSeparator rune = '\f'

type image struct {
	MIMEType string `json:"mimetype"`
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

			tempFile, err := os.Create(tempDir + "/input.pdf")
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

			pdfText, err := runPDFToText(tempDir + "/input.pdf", startPage, endPage)
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
