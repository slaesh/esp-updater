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

func getFileSize(path string) int64 {

	f, err := os.Open(path)
	if err != nil {
		return -1
	}

	defer f.Close() //Close after function return

	fStat, _ := f.Stat() //Get info from file
	//fSize := strconv.FormatInt(fStat.Size(), 10) //Get file size as a string
	//return fSize

	return fStat.Size()
}

func sendFile(w http.ResponseWriter, r *http.Request, path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "File not found.", 404)
		return -1, err
	}

	defer f.Close() //Close after function return

	filename := f.Name()
	fStat, _ := f.Stat()                         //Get info from file
	fSize := strconv.FormatInt(fStat.Size(), 10) //Get file size as a string

	md5sumStr, err := sumMd5(f)
	if err != nil {
		http.Error(w, "could not sum md5.", 400)
		return -1, err
	}

	// fmt.Println(filename, fSize, md5sumStr)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Header().Set("Content-Length", fSize)
	w.Header().Set("X-MD5", md5sumStr)

	// fmt.Println("sending file..")

	w.WriteHeader(200)

	f.Seek(0, 0)
	return io.Copy(w, f)
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

	highestVersion, _ := semver.Make("0.0.0")
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

func patchHeaderKey(key string, platform string) string {
	return strings.ReplaceAll(key, "#esp#", platform)
}

func update(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

	firmwaretype := ps.ByName("firmwaretype")

	/*
		map[
			Cache-Control:[no-cache]
			Connection:[close]
			User-Agent:[ESP32-http-Update]
			X-Esp32-Ap-Mac:[30:83:98:D1:A6:75]
			X-Esp32-Chip-Size:[8388608]
			X-Esp32-Free-Space:[1310720]
			X-Esp32-Mode:[sketch]
			X-Esp32-Sdk-Version:[v3.3.5-1-g85c43024c]
			X-Esp32-Sketch-Md5:[a82293a19146b14386acce8f01159d80]
			X-Esp32-Sketch-Sha256:[4805A8252F6551F80BF9A574D9D06D97329B22A492AE78E825661258F905AABD]
			X-Esp32-Sketch-Size:[1098688]
			X-Esp32-Sta-Mac:[30:83:98:D1:A6:74]
			X-Esp32-Version:[3.0.0]
		]
	*/

	userAgent := r.Header.Get("User-Agent")
	espPlatform := strings.Split(userAgent, "-")[0]

	mac := r.Header.Get(patchHeaderKey("X-#esp#-Sta-Mac", espPlatform))
	chipSize := r.Header.Get(patchHeaderKey("X-#esp#-Chip-Size", espPlatform))
	sketchSize := r.Header.Get(patchHeaderKey("X-#esp#-Sketch-Size", espPlatform))
	freeSpace := r.Header.Get(patchHeaderKey("X-#esp#-Free-Space", espPlatform))
	sdkVersion := r.Header.Get(patchHeaderKey("X-#esp#-Sdk-Version", espPlatform))
	version := r.Header.Get(patchHeaderKey("X-#esp#-Version", espPlatform))

	logPrefix := fmt.Sprintf("[%s//%s] ", espPlatform, mac)

	if !strings.HasPrefix(userAgent, "ESP") ||
		!strings.HasSuffix(userAgent, "-http-Update") ||
		mac == "" ||
		chipSize == "" ||
		sketchSize == "" ||
		freeSpace == "" ||
		sdkVersion == "" ||
		version == "" {

		fmt.Println(logPrefix+"invalid request: ", r.Header)

		w.WriteHeader(403 /* no esp8266 */)
		return
	}

	fmt.Println(logPrefix+"checking for an update..", firmwaretype, version)
	fmt.Println(logPrefix+"chipsize", chipSize, "sketchSize", sketchSize, "freeSpace", freeSpace)

	versionArduino, err := semver.Make(version)
	//fmt.Println(versionArduino, err)
	if err != nil {
		fmt.Println(logPrefix + "invalid version, no semver!")

		w.WriteHeader(304 /* no update present.. ! */)
		return
	}

	fwFolder := "./fw/" + firmwaretype
	fwFile, err := getHighestVersion(fwFolder)
	// fmt.Println(fwFolder, fwFile, err)
	if err != nil {
		fmt.Println(logPrefix+"cant find a firmware file", err)

		w.WriteHeader(304 /* no update present.. ! */)
		return
	}

	versionServer, err := filenameToSemver(fwFile)
	//fmt.Println(versionServer, err)
	if err != nil {
		fmt.Println(logPrefix+"firmware file has no valid semver syntax", err)

		w.WriteHeader(304 /* no update present.. ! */)
		return
	}

	fileSize := getFileSize(fwFolder + "/" + fwFile)
	freeSize, _ := strconv.ParseInt(freeSpace, 10, 64)

	fmt.Println(logPrefix+"latest firmware found with version", versionServer, "size", fileSize)
	if fileSize > freeSize {

		fmt.Println(logPrefix+"firmware file is to big?", fileSize, freeSize)

		w.WriteHeader(304 /* no update present.. ! */)
		return
	}

	// -1: v1 is less than v2
	//  0: equal
	//  1: v1 is greater than v2

	result := versionArduino.Compare(versionServer)
	// fmt.Println(result)
	if result == -1 {
		fmt.Println(logPrefix + "send our update!!")

		written, err := sendFile(w, r, fwFolder+"/"+fwFile)

		if err != nil {

			fmt.Println(logPrefix+"something went wrong..", err)
		} else {

			fmt.Println(logPrefix+"done!", written)
		}

		return
	} else if result == 1 {
		fmt.Println(logPrefix + "device version is newer than our last firmware file found?! Oo")
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
