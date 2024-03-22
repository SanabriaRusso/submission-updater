package main

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func InitializeS3Session(region string) (*s3.Client, error) {
	awsCfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("error loading AWS configuration: %w", err)
	}

	client := s3.NewFromConfig(awsCfg)
	return client, nil
}

func (ctx *AppContext) addMissingBlocksFromS3(submissions []Submission, appCfg AppConfig) []Submission {
	for i, sub := range submissions {
		if len(sub.RawBlock) == 0 {
			blockPath := appCfg.NetworkName + "/blocks/" + sub.BlockHash + ".dat"
			result, err := ctx.S3Session.GetObject(context.TODO(), &s3.GetObjectInput{
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
