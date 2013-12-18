package main

import (
	"errors"
	//"time"
	"io"
	"launchpad.net/goamz/aws"
	"log"
	"strconv"
)

type S3ProviderConfig struct {
	BaseProviderConfig

	AWSAccessKeyId     string `json:"aws_access_key_id"`
	AWSSecretAccessKey string `json:"aws_secret_access_key"`
	AWSS3Bucket        string `json:"aws_s3_bucket"`
	AWSS3Path          string `json:"aws_s3_path"`
	AWSS3StorageClass  string `json:"aws_s3_storage_class"`
}

func NewS3ProviderConfig(base BaseProviderConfig, data map[string]interface{}) (*S3ProviderConfig, error) {
	config := S3ProviderConfig{}

	config.BaseProviderConfig = base

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

	aws_s3_storage_class, ok := data["aws_s3_storage_class"]
	if ok {
		config.AWSS3StorageClass, ok = aws_s3_storage_class.(string)
		if !ok {
			return nil, errors.New("aws_s3_storage_class must be a string.")
		}
		if config.AWSS3StorageClass != "REDUCED_REDUNDANCY" && config.AWSS3StorageClass != "STANDARD" {
			log.Printf("WARNING: aws_s3_storage_class is not 'REDUCED_REDUNDANCY' or 'STANDARD'. Undefined behaviour may result.")
		}
	} else {
		config.AWSS3StorageClass = "REDUCED_REDUNDANCY"
	}

	return &config, nil
}

type S3Provider struct {
	BaseProvider

	bucket *Bucket
}

func (c S3ProviderConfig) NewProvider() (Provider, error) {
	p := &S3Provider{BaseProvider: BaseProvider{c}}

	auth := aws.Auth{
		AccessKey: c.AWSAccessKeyId,
		SecretKey: c.AWSSecretAccessKey,
	}

	s := NewS3(auth, aws.USEast)
	p.bucket = s.Bucket(c.AWSS3Bucket)

	return p, nil
}

type S3Object struct {
	BaseObject

	size   int64
	reader io.ReadCloser
}

func (s *S3Object) GetSize() (int64, error) {
	return s.size, nil
}

func (s *S3Object) Read(buf []byte) (int, error) {
	return s.reader.Read(buf)
}

func (s *S3Object) Close() error {
	return s.reader.Close()
}

func (p *S3Provider) GetConfig() S3ProviderConfig {
	return p.config.(S3ProviderConfig)
}

func (p *S3Provider) Get(id string) (Object, error) {
	panic("nope")
	path := p.GetConfig().AWSS3Path + id
	req := &S3Request{
		bucket: p.bucket.Name,
		path:   path,
	}
	err := p.bucket.prepare(req)
	if err != nil {
		return nil, err
	}
	hresp, err := p.bucket.run(req)

	if err != nil {
		return nil, err
	} else {
		return &S3Object{
			BaseObject: BaseObject{
				Metadata:   hresp.Header.Get("x-amz-meta-till"),
				identifier: id,
				exists:     true,
				provider:   p,
			},
			reader: hresp.Body,
			size:   hresp.ContentLength,
		}, nil
	}
}

func (p *S3Provider) GetURL(id string) (Object, error) {
	return nil, nil
}

func (p *S3Provider) Put(o Object) (Object, error) {
	panic("nope")
	path := p.GetConfig().AWSS3Path + o.GetBaseObject().identifier
	size, err := o.GetSize()

	if err != nil {
		return nil, err
	} else {
		headers := map[string][]string{
			"Content-Length":      {strconv.FormatInt(size, 10)},
			"Content-Type":        {"application/octet-stream"},
			"x-amz-acl":           {string(Private)},
			"x-amz-storage-class": {p.GetConfig().AWSS3StorageClass},
		}
		md := o.GetBaseObject().Metadata
		if len(md) > 0 {
			headers["x-amz-meta-till"] = []string{md}
		}

		req := &S3Request{
			method:  "PUT",
			bucket:  p.bucket.Name,
			path:    path,
			headers: headers,
			payload: o,
		}
		err := p.bucket.S3.Query(req, nil)

		if err != nil {
			log.Printf("Could not put file: %v", err)
			return nil, err
		} else {
			return nil, nil
		}
	}
}

func (p *S3Provider) Update(o Object) (Object, error) {
	//  TODO: Update the mod time on the S3 object.
	return nil, nil
}
