package main

import (
	"image"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// fyneImage e so um apelido para deixar as assinaturas dos componentes curtas.
type fyneImage = image.Image

// Cache em memoria das fotos (avatares, miniaturas). Linhas de lista sao
// reciclada o tempo todo; sem cache cada rolagem baixaria as fotos de novo.
var (
	imageCache   sync.Map // url -> image.Image
	imageLoading sync.Map // url -> *sync.Once (evita baixar duas vezes ao mesmo tempo)
)

// loadImageCached entrega a imagem pelo callback, sempre fora da thread de UI
// (o callback deve usar fyne.Do para tocar em widgets).
func loadImageCached(rawURL string, done func(fyneImage)) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || done == nil {
		return
	}
	if cached, ok := imageCache.Load(rawURL); ok {
		go done(cached.(image.Image))
		return
	}
	go func() {
		onceValue, _ := imageLoading.LoadOrStore(rawURL, &sync.Once{})
		once := onceValue.(*sync.Once)
		once.Do(func() {
			img, err := fetchImage(rawURL)
			if err == nil && img != nil {
				imageCache.Store(rawURL, img)
				return
			}
			// Falhou (internet caiu): solta a trava para a proxima tentativa
			// buscar de novo. Sem isto a foto so voltava reabrindo o app.
			imageLoading.Delete(rawURL)
		})
		if cached, ok := imageCache.Load(rawURL); ok {
			done(cached.(image.Image))
		}
	}()
}

func fetchImage(rawURL string) (image.Image, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, http.ErrMissingFile
	}
	img, _, err := image.Decode(io.LimitReader(res.Body, 12*1024*1024))
	return img, err
}
