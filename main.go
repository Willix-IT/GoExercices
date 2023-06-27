package main

import (
	"flag"
	"fmt"
	"image"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/disintegration/imaging"
)

type ImageFilter func(srcPath, destPath string) error

var filterFuncs = map[string]ImageFilter{
	"grayscale": applyGrayscaleFilter,
	"blur":      applyBlurFilter,
}

func applyGrayscaleFilter(srcPath, destPath string) error {
	return applyFilter(srcPath, destPath, imaging.Grayscale)
}

func applyBlurFilter(srcPath, destPath string) error {
	return applyFilter(srcPath, destPath, func(srcImage image.Image) *image.NRGBA {
		return imaging.Blur(srcImage, 5.0)
	})
}

func applyFilter(srcPath, destPath string, filterFunc func(srcImage image.Image) *image.NRGBA) error {
	srcImage, err := imaging.Open(srcPath)
	if err != nil {
		return err
	}

	filteredImage := filterFunc(srcImage)

	err = imaging.Save(filteredImage, destPath)
	if err != nil {
		return err
	}

	return nil
}

func iterateAndApplyFilter(srcPath, destPath string, filter ImageFilter, fileInfos []os.FileInfo, done chan string, errs chan error) {
	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() {
			continue
		}

		srcFilePath := filepath.Join(srcPath, fileInfo.Name())
		destFilePath := filepath.Join(destPath, fileInfo.Name())

		err := filter(srcFilePath, destFilePath)
		if err != nil {
			errs <- fmt.Errorf("Error applying filter to %s: %w", fileInfo.Name(), err)
			continue
		}

		done <- fileInfo.Name()
	}
}

func processImagesWithWaitGroup(srcPath, destPath string, filter ImageFilter, fileInfos []os.FileInfo) {
	var wg sync.WaitGroup
	done := make(chan string)
	errs := make(chan error)

	wg.Add(1)
	go func() {
		defer wg.Done()
		iterateAndApplyFilter(srcPath, destPath, filter, fileInfos, done, errs)
	}()

	go func() {
		wg.Wait()
		close(done)
		close(errs)
	}()

	for {
		select {
		case fileName, ok := <-done:
			if ok {
				fmt.Printf("Finished processing: %s\n", fileName)
			}
		case err, ok := <-errs:
			if ok {
				fmt.Println(err)
			}
		default:
			return
		}
	}
}

func processImagesWithChannel(srcPath, destPath string, filter ImageFilter, fileInfos []os.FileInfo) {
	done := make(chan string)
	errs := make(chan error)

	for _, fileInfo := range fileInfos {
		go func(fileInfo os.FileInfo) {
			srcFilePath := filepath.Join(srcPath, fileInfo.Name())
			destFilePath := filepath.Join(destPath, fileInfo.Name())

			err := filter(srcFilePath, destFilePath)
			if err != nil {
				errs <- fmt.Errorf("Error applying filter to %s: %w", fileInfo.Name(), err)
				return
			}

			done <- fileInfo.Name()
		}(fileInfo)
	}

	for {
		select {
		case fileName, ok := <-done:
			if ok {
				fmt.Printf("Finished processing: %s\n", fileName)
			}
		case err, ok := <-errs:
			if ok {
				fmt.Println(err)
			}
		default:
			return
		}
	}
}

func main() {
	srcPath := flag.String("src", "", "Source folder containing the images")
	destPath := flag.String("dst", "", "Destination folder to save the filtered images")
	filterStr := flag.String("filter", "", "Filter to apply (grayscale or blur)")
	task := flag.String("task", "", "Task method to use (waitgrp or channel)")

	flag.Parse()

	if *srcPath == "" || *destPath == "" || *filterStr == "" || *task == "" {
		fmt.Println("Usage: imggo -src <source_folder> -dst <destination_folder> -filter <filter_type> -task <task_method>")
		return
	}

	filter, ok := filterFuncs[*filterStr]
	if !ok {
		fmt.Printf("Invalid filter: %s\n", *filterStr)
		return
	}

	fileInfos, err := ioutil.ReadDir(*srcPath)
	if err != nil {
		fmt.Printf("Error reading directory: %s\n", err.Error())
		return
	}

	taskFuncs := map[string]func(string, string, ImageFilter, []os.FileInfo){
		"waitgrp": processImagesWithWaitGroup,
		"channel": processImagesWithChannel,
	}

	taskFunc, ok := taskFuncs[*task]
	if !ok {
		fmt.Printf("Invalid task method: %s\n", *task)
		return
	}

	taskFunc(*srcPath, *destPath, filter, fileInfos)
}
