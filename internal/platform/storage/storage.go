// Package storage keeps the uploaded files in a private S3 bucket. Browsers
// send and fetch them through presigned requests, so no file goes through
// the Lambda.
package storage

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Bucket struct {
	client  *s3.Client
	presign *s3.PresignClient
	name    string
}

func New(client *s3.Client, name string) *Bucket {
	return &Bucket{client: client, presign: s3.NewPresignClient(client), name: name}
}

// Upload is an HTML form POST: the browser sends Fields plus the file. The
// policy pins the key, the content type and the size range, so S3 itself
// rejects anything else.
type Upload struct {
	URL       string
	Fields    map[string]string
	ExpiresAt time.Time
}

func (b *Bucket) PresignUpload(ctx context.Context, key, contentType string, maxBytes int64, ttl time.Duration) (Upload, error) {
	req, err := b.presign.PresignPostObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.name),
		Key:    aws.String(key),
	}, func(o *s3.PresignPostOptions) {
		o.Expires = ttl
		o.Conditions = []interface{}{
			[]interface{}{"eq", "$Content-Type", contentType},
			[]interface{}{"content-length-range", 1, maxBytes},
		}
	})
	if err != nil {
		return Upload{}, err
	}
	req.Values["Content-Type"] = contentType
	return Upload{URL: req.URL, Fields: req.Values, ExpiresAt: time.Now().Add(ttl)}, nil
}

// PresignDownload makes the browser save the file under fileName.
func (b *Bucket) PresignDownload(ctx context.Context, key, fileName string, ttl time.Duration) (string, error) {
	req, err := b.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     aws.String(b.name),
		Key:                        aws.String(key),
		ResponseContentDisposition: aws.String("attachment; filename*=UTF-8''" + url.PathEscape(fileName)),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// Size reports found=false when the object does not exist.
func (b *Bucket) Size(ctx context.Context, key string) (size int64, found bool, err error) {
	out, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)})
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return aws.ToInt64(out.ContentLength), true, nil
}

func (b *Bucket) Delete(ctx context.Context, key string) error {
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)})
	return err
}

// DeletePrefix deletes every object under prefix. Listing needs
// s3:ListBucket, which also makes HeadObject answer 404 instead of 403 for a
// missing key.
func (b *Bucket) DeletePrefix(ctx context.Context, prefix string) error {
	pages := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{Bucket: aws.String(b.name), Prefix: aws.String(prefix)})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return err
		}
		if len(page.Contents) == 0 {
			continue
		}
		ids := make([]types.ObjectIdentifier, len(page.Contents))
		for i, o := range page.Contents {
			ids[i] = types.ObjectIdentifier{Key: o.Key}
		}
		out, err := b.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(b.name),
			Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return err
		}
		if len(out.Errors) > 0 {
			first := out.Errors[0]
			return fmt.Errorf("%d objects not deleted, %s: %s", len(out.Errors), aws.ToString(first.Key), aws.ToString(first.Message))
		}
	}
	return nil
}
