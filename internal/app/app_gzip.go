package app

import (
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type gzWriter struct {
	gin.ResponseWriter
	Writer io.Writer
}

func (w gzWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func (w gzWriter) WriteString(s string) (int, error) {
	return w.Writer.Write([]byte(s))
}

// избегаем попадания заголовков в gzWriter
func (w gzWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

// избегаем попадания заголовков в gzWriter
func (w gzWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
}

var gzipReader *gzip.Reader
var gzipWriter *gzip.Writer

func (app App) middlewareGzipCompression(c *gin.Context) {
	logger := app.Logger

	// разжимаем запрос
	ce := c.GetHeader("Content-Encoding")
	switch {
	case ce == "gzip":
		var err error
		if gzipReader == nil {
			gzipReader, err = gzip.NewReader(c.Request.Body)
		} else {
			err = gzipReader.Reset(c.Request.Body)
		}
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}
		defer gzipReader.Close()
		c.Request.Body = gzipReader
	case ce == "deflate":
		// в документации написано, что io.ReadCloser, который возвращает zlib.NewReader,
		// имплементирует интерфейс Resetter с методом Reset - но кажется это не так :(
		// а без Reset смысла в кешировании глобальной переменной нет
		//err := zlibReader.Reset(c.Request.Body)
		zlibReader, err := zlib.NewReader(c.Request.Body)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
		}
		defer zlibReader.Close()
		c.Request.Body = zlibReader
	case len(ce) > 0:
		c.String(http.StatusBadRequest, "unsupported Content-Encoding")
		return
	}

	if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
		c.Next()
		return
	}

	// жмём ответ
	switch c.GetHeader("Content-Type") {
	case "application/json", "application/javascript", "text/plain", "text/html", "text/css", "text/xml":
		if gzipWriter == nil {
			var err error
			gzipWriter, err = gzip.NewWriterLevel(c.Writer, gzip.BestCompression)
			if err != nil {
				logger.Fatal().Err(err)
				break
			}
		} else {
			gzipWriter.Reset(c.Writer)
		}
		defer gzipWriter.Close()

		c.Writer = gzWriter{c.Writer, gzipWriter}
		c.Header("Content-Encoding", "gzip")
	}

	c.Next()
}
