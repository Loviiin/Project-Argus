package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const version = "1.24.1"

func main() {
	fmt.Println(">> Setting up ONNX Runtime version", version)

	var osName, arch, ext string

	switch runtime.GOOS {
	case "windows":
		osName = "win"
		ext = "zip"
	case "darwin":
		osName = "osx"
		ext = "tgz"
	case "linux":
		osName = "linux"
		ext = "tgz"
	default:
		fmt.Println("Unsupported OS:", runtime.GOOS)
		os.Exit(1)
	}

	switch runtime.GOARCH {
	case "amd64":
		arch = "x64"
		if runtime.GOOS == "darwin" {
			arch = "x86_64"
		}
	case "arm64":
		arch = "aarch64"
		if runtime.GOOS == "darwin" {
			arch = "arm64"
		}
	default:
		fmt.Println("Unsupported Arch:", runtime.GOARCH)
		os.Exit(1)
	}

	url := fmt.Sprintf("https://github.com/microsoft/onnxruntime/releases/download/v%s/onnxruntime-%s-%s-%s.%s", version, osName, arch, version, ext)
	fmt.Printf(">> Downloading from: %s\n", url)

	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("onnxruntime.%s", ext))
	if err := downloadFile(tmpFile, url); err != nil {
		fmt.Println("Download failed:", err)
		os.Exit(1)
	}
	defer os.Remove(tmpFile)

	fmt.Println(">> Extracting library...")
	var targetLibrary string
	if runtime.GOOS == "windows" {
		targetLibrary = "onnxruntime.dll"
	} else if runtime.GOOS == "darwin" {
		targetLibrary = "libonnxruntime.dylib"
	} else {
		targetLibrary = "libonnxruntime.so"
	}

	err := extractLibrary(tmpFile, ext, targetLibrary)
	if err != nil {
		fmt.Println("Extraction failed:", err)
		os.Exit(1)
	}

	fmt.Println(">> Success! Installed", targetLibrary)
}

func downloadFile(filepath string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func extractLibrary(archivePath, ext, destName string) error {
	if ext == "zip" {
		r, err := zip.OpenReader(archivePath)
		if err != nil {
			return err
		}
		defer r.Close()

		for _, f := range r.File {
			if strings.HasSuffix(f.Name, "onnxruntime.dll") {
				rc, err := f.Open()
				if err != nil {
					return err
				}
				defer rc.Close()

				out, err := os.Create(destName)
				if err != nil {
					return err
				}
				defer out.Close()

				_, err = io.Copy(out, rc)
				return err
			}
		}
	} else if ext == "tgz" {
		f, err := os.Open(archivePath)
		if err != nil {
			return err
		}
		defer f.Close()

		gr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gr.Close()

		tr := tar.NewReader(gr)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}

			fileName := filepath.Base(hdr.Name)
			if hdr.Typeflag == tar.TypeReg && !strings.Contains(hdr.Name, "providers") {
				if (destName == "libonnxruntime.so" && strings.HasPrefix(fileName, "libonnxruntime.so")) ||
					(destName == "libonnxruntime.dylib" && strings.HasPrefix(fileName, "libonnxruntime.") && strings.HasSuffix(fileName, ".dylib")) {
					out, err := os.Create(destName)
					if err != nil {
						return err
					}
					defer out.Close()
					_, err = io.Copy(out, tr)
					return err
				}
			}
		}
	}
	return fmt.Errorf("library not found in archive")
}
