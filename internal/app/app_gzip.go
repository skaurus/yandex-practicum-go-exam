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

// Header - we need this to avoid gzipping HTTP headers
func (w gzWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

// WriteHeader - we need this to avoid gzipping HTTP headers
func (w gzWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
}

var gzipReader *gzip.Reader
var gzipWriter *gzip.Writer

func (runEnv Env) middlewareGzipCompression(c *gin.Context) {
	logger := runEnv.Logger()

	// reading gzipped request
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
		// documentation states that io.ReadCloser, as returned by
		// zlib.NewReader, does implement Resetter interface and so should have
		// Reset method - but it seems not so. And without Reset we can't use
		// global variable and save on its initializing.
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

	// writing gzipped answer
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
