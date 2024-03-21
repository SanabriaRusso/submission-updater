package main

import (
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

func InitializeS3Session(region string) (*s3.S3, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region)},
	)
	if err != nil {
		return nil, fmt.Errorf("error initializing AWS session: %w", err)
	}

	return s3.New(sess), nil
}

func (ctx *Context) addMissingBlocksFromS3(submissions []Submission, appCfg AppConfig) []Submission {
	for i, sub := range submissions {
		if len(sub.RawBlock) == 0 {
			blockPath := appCfg.NetworkName + "/blocks/" + sub.BlockHash + ".dat"
			result, err := ctx.S3Session.GetObject(&s3.GetObjectInput{
				Bucket: aws.String(appCfg.AwsConfig.BucketName),
				Key:    aws.String(blockPath),
			})
			if err != nil {
				ctx.Log.Errorf("Failed to get object for block hash %s from S3: %v", sub.BlockHash, err)
				// Continue to next submission instead of stopping
				continue
			}

			rawBlock, err := io.ReadAll(result.Body)
			if err != nil {
				ctx.Log.Errorf("Failed to read body for block hash %s from S3 object: %v", sub.BlockHash, err)
				// Continue to next submission instead of stopping
				continue
			}

			// Update submission with the retrieved block
			// if the block was successfully read
			// if block not read, assign empty byte slice
			if len(rawBlock) > 0 {
				submissions[i].RawBlock = rawBlock
			} else {
				submissions[i].RawBlock = []byte(`""`)
			}
		}
	}

	return submissions
}
