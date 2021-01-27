package main

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/julienschmidt/httprouter"
)

func sumMd5(f *os.File) (string, error) {
	hash := md5.New()

	f.Seek(0, 0)
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}

	hashInBytes := hash.Sum(nil)[:16]

	return hex.EncodeToString(hashInBytes), nil
}

func sendFile(w http.ResponseWriter, r *http.Request, path string) {
	f, err := os.Open(path)
	defer f.Close() //Close after function return

	if err != nil {
		http.Error(w, "File not found.", 404)
		return
	}

	filename := f.Name()
	fStat, _ := f.Stat()                         //Get info from file
	fSize := strconv.FormatInt(fStat.Size(), 10) //Get file size as a string

	md5sumStr, err := sumMd5(f)
	if err != nil {
		http.Error(w, "could not sum md5.", 400)
		return
	}

	fmt.Println(filename, fSize, md5sumStr)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Header().Set("Content-Length", fSize)
	w.Header().Set("X-MD5", md5sumStr)

	fmt.Println("sending file..")

	w.WriteHeader(200)

	f.Seek(0, 0)
	io.Copy(w, f)
}

func index(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	fmt.Fprint(w, "Welcome!\n")
}

// ReadDir reads the directory named by dirname and returns
// a list of directory entries sorted by filename.
func ReadDir(dirname string) ([]os.FileInfo, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	list, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name() < list[j].Name() })
	return list, nil
}

func filenameToSemver(filename string) (semver.Version, error) {
	filePath := strings.ToLower(filename)
	fileExt := filepath.Ext(filePath)
	fNameWoExt := strings.Split(filePath, fileExt)[0]

	version, err := semver.Make(fNameWoExt)
	if err != nil {
		return version, fmt.Errorf("could not create semver: %s", fNameWoExt)
	}

	return version, nil
}

func getHighestVersion(dirname string) (string, error) {

	files, err := ReadDir(dirname)
	if err != nil {
		return "", err
	}

	highestVersion, err := semver.Make("0.0.0")
	highestFile := ""

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		curVersion, err := filenameToSemver(f.Name())
		if err != nil {
			continue
		}

		if curVersion.Compare(highestVersion) >= 0 {
			highestVersion = curVersion
			highestFile = f.Name()
		}
	}

	if highestFile == "" {
		return "", errors.New("no firmware found at all")
	}

	return highestFile, nil
}

func update(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

	firmwaretype := ps.ByName("firmwaretype")

	userAgent := r.Header.Get("User-Agent")
	mac := r.Header.Get("X-Esp8266-Sta-Mac")
	chipSize := r.Header.Get("X-Esp8266-Chip-Size")
	sketchSize := r.Header.Get("X-Esp8266-Sketch-Size")
	freeSpace := r.Header.Get("X-Esp8266-Free-Space")
	sdkVersion := r.Header.Get("X-Esp8266-Sdk-Version")
	version := r.Header.Get("X-Esp8266-Version")

	if userAgent != "ESP8266-http-Update" ||
		mac == "" ||
		chipSize == "" ||
		sketchSize == "" ||
		freeSpace == "" ||
		sdkVersion == "" ||
		version == "" {
		w.WriteHeader(403 /* no esp8266 */)
		fmt.Println("invalid request: ", r.Header)
		return
	}

	fmt.Println("check for update..", mac, version)

	versionArduino, err := semver.Make(version)
	fmt.Println(versionArduino, err)

	if err == nil {
		fwFolder := "./fw/" + firmwaretype
		fwFile, err := getHighestVersion(fwFolder)
		fmt.Println(fwFolder, fwFile, err)

		if err == nil {
			versionServer, err := filenameToSemver(fwFile)
			fmt.Println(versionServer, err)

			if err == nil {
				// -1: v1 is less than v2
				//  0: equal
				//  1: v1 is greater than v2

				result := versionArduino.Compare(versionServer)
				fmt.Println(result)
				if result == -1 {
					fmt.Println("we need to serve an update!!")

					sendFile(w, r, fwFolder+"/"+fwFile)
					return
				}
			}
		}
	}

	w.WriteHeader(304 /* no update present.. ! */)
}

func main() {
	router := httprouter.New()

	router.GET("/", index)
	router.GET("/update/:firmwaretype", update)

	fmt.Println("serving on port 35982")

	if err := http.ListenAndServe(":35982", router); err != nil {
		log.Fatal(err)
	}
}
