package cache

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/ulikunitz/xz"
)

// Compressor handles compression and decompression of packages
type Compressor struct {
	encoder *zstd.Encoder
	decoder *zstd.Decoder
}

// NewCompressor creates a new compressor instance
func NewCompressor() (*Compressor, error) {
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		encoder.Close()
		return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
	}

	return &Compressor{
		encoder: encoder,
		decoder: decoder,
	}, nil
}

// Close closes the compressor and releases resources
func (c *Compressor) Close() {
	if c.encoder != nil {
		c.encoder.Close()
	}
	if c.decoder != nil {
		c.decoder.Close()
	}
}

// CompressFile compresses a single file to a tar.zst archive
func (c *Compressor) CompressFile(srcPath, destPath string) error {
	log.Infof("Compressing %s to %s", srcPath, destPath)

	// Create destination directory if it doesn't exist
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Open source file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	// Get source file info
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get source file info: %w", err)
	}

	// Create destination file
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	// Create zstd writer
	zstdWriter := c.encoder
	zstdWriter.Reset(destFile)
	defer zstdWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(zstdWriter)
	defer tarWriter.Close()

	// Add file to tar archive
	header := &tar.Header{
		Name: filepath.Base(srcPath),
		Mode: int64(srcInfo.Mode()),
		Size: srcInfo.Size(),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}

	if _, err := io.Copy(tarWriter, srcFile); err != nil {
		return fmt.Errorf("failed to write file to tar: %w", err)
	}

	log.Infof("Successfully compressed %s to %s", srcPath, destPath)
	return nil
}

// CompressDirectory compresses a directory to a tar.zst archive
func (c *Compressor) CompressDirectory(srcDir, destPath string) error {
	log.Infof("Compressing directory %s to %s", srcDir, destPath)

	// Create destination directory if it doesn't exist
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Create destination file
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	// Create zstd writer
	zstdWriter := c.encoder
	zstdWriter.Reset(destFile)
	defer zstdWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(zstdWriter)
	defer tarWriter.Close()

	// Walk through the source directory
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Skip the root directory itself
		if relPath == "." {
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header: %w", err)
		}
		header.Name = relPath

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}

		// Write file content if it's a regular file
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", path, err)
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return fmt.Errorf("failed to write file to tar: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to compress directory: %w", err)
	}

	log.Infof("Successfully compressed directory %s to %s", srcDir, destPath)
	return nil
}

// DecompressFile decompresses a tar.zst archive to a destination directory
func (c *Compressor) DecompressFile(srcPath, destDir string) error {
	log.Infof("Decompressing %s to %s", srcPath, destDir)

	// Create destination directory if it doesn't exist
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Open source file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	// Create zstd reader
	zstdReader := c.decoder
	zstdReader.Reset(srcFile)

	// Create tar reader
	tarReader := tar.NewReader(zstdReader)

	// Extract files
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Sanitize the file path to prevent directory traversal
		destPath := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(destPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(destPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", destPath, err)
			}
		case tar.TypeReg:
			// Create file
			destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", destPath, err)
			}

			if _, err := io.Copy(destFile, tarReader); err != nil {
				destFile.Close()
				return fmt.Errorf("failed to write file %s: %w", destPath, err)
			}
			destFile.Close()
		}
	}

	log.Infof("Successfully decompressed %s to %s", srcPath, destDir)
	return nil
}

// NormalizeToTarZst normalizes any file format to tar.zst format
func (c *Compressor) NormalizeToTarZst(srcPath, destPath string, format FileFormat) error {
	log.Infof("Normalizing %s (%s) to %s", srcPath, format, destPath)

	// Create destination directory if it doesn't exist
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	switch format {
	case FileFormatTar:
		// Case 1: .tar file - directly compress with zstd
		return c.compressTarWithZstd(srcPath, destPath)

	case FileFormatTarGz, FileFormatTgz:
		// Case 2: .tar.gz/.tgz file - decompress to .tar then compress with zstd
		return c.decompressAndRecompress(srcPath, destPath, "gzip")

	case FileFormatTarBz2:
		// Case 2: .tar.bz2 file - decompress to .tar then compress with zstd
		return c.decompressAndRecompress(srcPath, destPath, "bzip2")

	case FileFormatTarXz:
		// Case 2: .tar.xz file - decompress to .tar then compress with zstd
		return c.decompressAndRecompress(srcPath, destPath, "xz")

	case FileFormatBinary:
		// Case 3: binary file - create .tar then compress with zstd
		return c.createTarAndCompress(srcPath, destPath)

	default:
		return fmt.Errorf("unsupported file format: %s", format)
	}
}

// compressTarWithZstd compresses a .tar file directly with zstd
func (c *Compressor) compressTarWithZstd(srcPath, destPath string) error {
	log.Infof("Compressing tar file %s with zstd to %s", srcPath, destPath)

	// Open source tar file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source tar file: %w", err)
	}
	defer srcFile.Close()

	// Create destination file
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	// Create zstd writer
	zstdWriter := c.encoder
	zstdWriter.Reset(destFile)
	defer zstdWriter.Close()

	// Copy tar content through zstd compression
	if _, err := io.Copy(zstdWriter, srcFile); err != nil {
		return fmt.Errorf("failed to compress tar file: %w", err)
	}

	log.Infof("Successfully compressed tar file to %s", destPath)
	return nil
}

// decompressAndRecompress decompresses a compressed tar file and recompresses with zstd
func (c *Compressor) decompressAndRecompress(srcPath, destPath, compressionType string) error {
	log.Infof("Decompressing %s (%s) and recompressing with zstd to %s", srcPath, compressionType, destPath)

	// Open source file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	// Create appropriate decompressor
	var reader io.Reader
	switch compressionType {
	case "gzip":
		gzipReader, err := gzip.NewReader(srcFile)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzipReader.Close()
		reader = gzipReader

	case "bzip2":
		reader = bzip2.NewReader(srcFile)

	case "xz":
		xzReader, err := xz.NewReader(srcFile)
		if err != nil {
			return fmt.Errorf("failed to create xz reader: %w", err)
		}
		reader = xzReader
	default:
		return fmt.Errorf("unsupported compression type: %s", compressionType)
	}

	// Create destination file
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	// Create zstd writer
	zstdWriter := c.encoder
	zstdWriter.Reset(destFile)
	defer zstdWriter.Close()

	// Copy decompressed tar content through zstd compression
	if _, err := io.Copy(zstdWriter, reader); err != nil {
		return fmt.Errorf("failed to recompress with zstd: %w", err)
	}

	log.Infof("Successfully decompressed and recompressed to %s", destPath)
	return nil
}

// createTarAndCompress creates a tar archive from a binary file and compresses with zstd
func (c *Compressor) createTarAndCompress(srcPath, destPath string) error {
	log.Infof("Creating tar archive from binary %s and compressing to %s", srcPath, destPath)

	// Open source file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	// Get source file info
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get source file info: %w", err)
	}

	// Create destination file
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	// Create zstd writer
	zstdWriter := c.encoder
	zstdWriter.Reset(destFile)
	defer zstdWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(zstdWriter)
	defer tarWriter.Close()

	// Add file to tar archive
	header := &tar.Header{
		Name: filepath.Base(srcPath),
		Mode: int64(srcInfo.Mode()),
		Size: srcInfo.Size(),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}

	if _, err := io.Copy(tarWriter, srcFile); err != nil {
		return fmt.Errorf("failed to write file to tar: %w", err)
	}

	log.Infof("Successfully created tar archive and compressed to %s", destPath)
	return nil
}
