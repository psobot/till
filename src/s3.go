package main

import (
	"errors"
)

type S3ProviderConfig struct {
	BaseProviderConfig

	AWSUserName        string `json:"aws_user_name"`
	AWSAccessKeyId     string `json:"aws_access_key_id"`
	AWSSecretAccessKey string `json:"aws_secret_access_key"`
	AWSS3Bucket        string `json:"aws_s3_bucket"`
	AWSS3Path          string `json:"aws_s3_path"`
}

func NewS3ProviderConfig(base BaseProviderConfig, data map[string]interface{}) (*S3ProviderConfig, error) {
	config := S3ProviderConfig{}

	config.BaseProviderConfig = base

	username, ok := data["aws_user_name"]
	if ok {
		config.AWSUserName, ok = username.(string)
		if !ok {
			return nil, errors.New("aws_user_name must be a string.")
		}
	} else {
		return nil, errors.New("aws_user_name must be defined.")
	}

	access_key_id, ok := data["aws_access_key_id"]
	if ok {
		config.AWSAccessKeyId, ok = access_key_id.(string)
		if !ok {
			return nil, errors.New("aws_access_key_id must be a string.")
		}
	} else {
		return nil, errors.New("aws_access_key_id must be defined.")
	}

	secret_access_key, ok := data["aws_secret_access_key"]
	if ok {
		config.AWSSecretAccessKey, ok = secret_access_key.(string)
		if !ok {
			return nil, errors.New("aws_secret_access_key must be a string.")
		}
	} else {
		return nil, errors.New("aws_secret_access_key must be defined.")
	}

	aws_s3_bucket, ok := data["aws_s3_bucket"]
	if ok {
		config.AWSS3Bucket, ok = aws_s3_bucket.(string)
		if !ok {
			return nil, errors.New("aws_s3_bucket must be a string.")
		}
	} else {
		return nil, errors.New("aws_s3_bucket must be defined.")
	}

	aws_s3_path, ok := data["aws_s3_path"]
	if ok {
		config.AWSS3Path, ok = aws_s3_path.(string)
		if !ok {
			return nil, errors.New("aws_s3_path must be a string.")
		}
	} else {
		config.AWSS3Path = "/"
	}

	return &config, nil
}
