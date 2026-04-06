package services

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

// R2Service handles Cloudflare R2 file storage
// Used for: submission files (pdf/doc/docx/jpg/png) and user avatars
// NOT used for: bounty terms, transaction data — those use IPFS
type R2Service struct {
	client     *s3.Client
	bucketName string
	publicURL  string
}

// AllowedMimeTypes maps extensions to magic bytes for file validation
var allowedMagicBytes = map[string][]byte{
	".pdf":  {0x25, 0x50, 0x44, 0x46}, // %PDF
	".jpg":  {0xFF, 0xD8, 0xFF},        // JPEG
	".jpeg": {0xFF, 0xD8, 0xFF},        // JPEG
	".png":  {0x89, 0x50, 0x4E, 0x47}, // PNG
	".doc":  {0xD0, 0xCF, 0x11, 0xE0}, // DOC (Compound Document)
	".docx": {0x50, 0x4B, 0x03, 0x04}, // DOCX (ZIP)
}

const (
	MaxSubmissionSize = 4_999_552 // 4.95 MB in bytes
	MaxAvatarSize     = 2_097_152 // 2 MB in bytes
)

// NewR2Service creates a new Cloudflare R2 service instance
func NewR2Service(accountID, accessKeyID, secretAccessKey, bucketName, publicURL string) (*R2Service, error) {
	r2Endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKeyID,
			secretAccessKey,
			"",
		)),
		config.WithRegion("auto"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load R2 config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(r2Endpoint)
		o.UsePathStyle = true
	})

	return &R2Service{
		client:     client,
		bucketName: bucketName,
		publicURL:  publicURL,
	}, nil
}

// R2UploadResult contains the result of a file upload
type R2UploadResult struct {
	Path          string // R2 object key (stored in DB)
	PublicURL     string // Public URL (if bucket is public)
	FileSize      int64
	FileType      string
	WorkHashSHA256 string // SHA-256 of path + size — used for on-chain proof
}

// ValidateFileBytes reads the first 8 bytes and checks magic numbers
// Returns the file extension if valid, error if not allowed
func ValidateFileBytes(file multipart.File, filename string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return "", fmt.Errorf("file has no extension")
	}

	magic, allowed := allowedMagicBytes[ext]
	if !allowed {
		return "", fmt.Errorf("file type %s not allowed. Allowed: pdf, doc, docx, jpg, jpeg, png", ext)
	}

	header := make([]byte, 8)
	n, err := file.Read(header)
	if err != nil || n < len(magic) {
		return "", fmt.Errorf("cannot read file header")
	}

	for i, b := range magic {
		if header[i] != b {
			return "", fmt.Errorf("file content does not match extension %s — possible file spoofing", ext)
		}
	}

	// Seek back to start
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("failed to seek file: %w", err)
	}

	return ext, nil
}

// UploadSubmission uploads a freelancer's submission file to R2
// Path format: bountyvault-submissions/{freelancer_id}/{bounty_id}/{sub_num}/{uuid}.{ext}
func (s *R2Service) UploadSubmission(
	ctx context.Context,
	freelancerID, bountyID string,
	submissionNumber int,
	file multipart.File,
	filename string,
	fileSize int64,
) (*R2UploadResult, error) {
	if fileSize > MaxSubmissionSize {
		return nil, fmt.Errorf("file too large: %d bytes. Maximum allowed: %d bytes (4.95 MB)", fileSize, MaxSubmissionSize)
	}

	ext, err := ValidateFileBytes(file, filename)
	if err != nil {
		return nil, err
	}

	fileID := uuid.New().String()
	objectKey := fmt.Sprintf("bountyvault-submissions/%s/%s/%d/%s%s",
		freelancerID, bountyID, submissionNumber, fileID, ext)

	contentType := getContentType(ext)

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucketName),
		Key:           aws.String(objectKey),
		Body:          file,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(fileSize),
	})
	if err != nil {
		return nil, fmt.Errorf("R2 upload failed: %w", err)
	}

	// Compute work hash: SHA256(r2_path + file_size) — stored on-chain
	hashInput := fmt.Sprintf("%s_%d", objectKey, fileSize)
	hash := sha256.Sum256([]byte(hashInput))
	workHashHex := fmt.Sprintf("%x", hash)

	return &R2UploadResult{
		Path:          objectKey,
		PublicURL:     fmt.Sprintf("%s/%s", s.publicURL, objectKey),
		FileSize:      fileSize,
		FileType:      strings.TrimPrefix(ext, "."),
		WorkHashSHA256: workHashHex,
	}, nil
}

// UploadAvatar uploads a user avatar to R2
// Path format: bountyvault-avatars/{user_id}/{uuid}.{ext}
func (s *R2Service) UploadAvatar(
	ctx context.Context,
	userID string,
	file multipart.File,
	filename string,
	fileSize int64,
) (*R2UploadResult, error) {
	if fileSize > MaxAvatarSize {
		return nil, fmt.Errorf("avatar too large: %d bytes. Maximum: 2 MB", fileSize)
	}

	ext, err := ValidateFileBytes(file, filename)
	if err != nil {
		return nil, err
	}
	// Only images for avatars
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return nil, fmt.Errorf("avatar must be jpg, jpeg, or png")
	}

	fileID := uuid.New().String()
	objectKey := fmt.Sprintf("bountyvault-avatars/%s/%s%s", userID, fileID, ext)
	contentType := getContentType(ext)

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucketName),
		Key:           aws.String(objectKey),
		Body:          file,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(fileSize),
	})
	if err != nil {
		return nil, fmt.Errorf("R2 avatar upload failed: %w", err)
	}

	return &R2UploadResult{
		Path:      objectKey,
		PublicURL: fmt.Sprintf("%s/%s", s.publicURL, objectKey),
		FileSize:  fileSize,
		FileType:  strings.TrimPrefix(ext, "."),
	}, nil
}

// GenerateSignedURL generates a pre-signed URL for private file access (24hr expiry)
func (s *R2Service) GenerateSignedURL(ctx context.Context, objectKey string) (string, error) {
	presignClient := s3.NewPresignClient(s.client)

	req, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(objectKey),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = 24 * time.Hour
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate signed URL: %w", err)
	}

	return req.URL, nil
}

// DeleteObject removes an object from R2 (e.g. on bounty deletion)
func (s *R2Service) DeleteObject(ctx context.Context, objectKey string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(objectKey),
	})
	return err
}

// UploadEncryptionKey uploads a .txt file containing the mega.nz decryption key to R2
// Path format: bountyvault-keys/{freelancer_id}/{bounty_id}/{sub_num}/{uuid}.txt
// Returns the R2 path and a SHA256 hash of (mega_link + r2_path) for on-chain proof
func (s *R2Service) UploadEncryptionKey(
	ctx context.Context,
	freelancerID, bountyID string,
	submissionNumber int,
	content []byte,
	megaNZLink string,
) (*R2UploadResult, error) {
	if len(content) > 1024 {
		return nil, fmt.Errorf("encryption key file too large: max 1KB")
	}

	fileID := uuid.New().String()
	objectKey := fmt.Sprintf("bountyvault-keys/%s/%s/%d/%s.txt",
		freelancerID, bountyID, submissionNumber, fileID)

	reader := strings.NewReader(string(content))
	contentLen := int64(len(content))

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucketName),
		Key:           aws.String(objectKey),
		Body:          reader,
		ContentType:   aws.String("text/plain"),
		ContentLength: aws.Int64(contentLen),
	})
	if err != nil {
		return nil, fmt.Errorf("R2 encryption key upload failed: %w", err)
	}

	// Compute work hash: SHA256(mega_nz_link + r2_path) — stored on-chain
	hashInput := fmt.Sprintf("%s_%s", megaNZLink, objectKey)
	hash := sha256.Sum256([]byte(hashInput))
	workHashHex := fmt.Sprintf("%x", hash)

	return &R2UploadResult{
		Path:           objectKey,
		PublicURL:      fmt.Sprintf("%s/%s", s.publicURL, objectKey),
		FileSize:       contentLen,
		FileType:       "txt",
		WorkHashSHA256: workHashHex,
	}, nil
}

func getContentType(ext string) string {
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	default:
		return http.DetectContentType([]byte{})
	}
}
