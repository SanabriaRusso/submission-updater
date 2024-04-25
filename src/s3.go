package main

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func InitializeS3Session(ctx context.Context, region string) (*s3.Client, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("error loading AWS configuration: %w", err)
	}

	client := s3.NewFromConfig(awsCfg)
	return client, nil
}

func (appCtx *AppContext) addMissingBlocksFromS3(ctx context.Context, submissions []Submission, appCfg AppConfig) []Submission {
	blockCache := make(map[string][]byte) // Cache for holding block data

	for i, sub := range submissions {
		if len(sub.RawBlock) == 0 {
			// Check if the block is already in the cache
			if rawBlock, found := blockCache[sub.BlockHash]; found {
				submissions[i].RawBlock = rawBlock
				continue
			}

			// Block not in cache, download it from S3
			blockPath := appCfg.NetworkName + "/blocks/" + sub.BlockHash + ".dat"
			result, err := appCtx.S3Session.GetObject(ctx, &s3.GetObjectInput{
				Bucket: aws.String(appCfg.AwsConfig.BucketName),
				Key:    aws.String(blockPath),
			})
			if err != nil {
				appCtx.Log.Errorf("Failed to get object for block hash %s from S3: %v", sub.BlockHash, err)
				// Continue to next submission instead of stopping
				continue
			}
			defer result.Body.Close()

			rawBlock, err := io.ReadAll(result.Body)
			if err != nil {
				appCtx.Log.Errorf("Failed to read body for block hash %s from S3 object: %v", sub.BlockHash, err)
				// Continue to next submission instead of stopping
				continue
			}

			// Update submission with the retrieved block
			// if the block was successfully read
			// if block not read, assign empty byte slice
			// also cache the block data for future use
			if len(rawBlock) > 0 {
				blockCache[sub.BlockHash] = rawBlock
				submissions[i].RawBlock = rawBlock
			} else {
				emptyBlock := []byte(`""`)
				blockCache[sub.BlockHash] = emptyBlock
				submissions[i].RawBlock = emptyBlock
			}
		}
	}

	return submissions
}
