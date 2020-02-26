// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2016 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	// "archive/tar"

	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	// "io/ioutil"
	"os"
	"path"

	// "path/filepath"
	"strings"
	"time"

	"code.gitea.io/gitea/models"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"

	"gitea.com/macaron/session"
	archiver "github.com/mholt/archiver/v3"
	"github.com/unknwon/cae/zip"
	"github.com/unknwon/com"
	"github.com/urfave/cli"
)

// type outWriter interface {
// 	addFile(w, filePath string, absPath string) error
// 	addDir(w, dirPath string, absPath string) error
// 	AddEmptyDir(dirPath string) bool
// 	Close() error
// }

// type tarWriter struct {
// 	Tar     *tar.Writer
// 	Verbose bool
// }

// func newtarWriter(writer io.Writer, verbose bool) *tarWriter {
// 	w := new(tarWriter)
// 	w.Verbose = verbose
// 	w.Tar = tar.NewWriter(writer)

// 	return w
// }

// func (w *tarWriter) addFile(w, filePath string, absPath string) error {
// 	f, err := os.Open(absPath)
// 	if err != nil {
// 		return fmt.Errorf("Unable to open %s: %s", absPath, err)
// 	}
// 	defer f.Close()

// 	stat, err := os.Stat(absPath)
// 	if err != nil {
// 		return fmt.Errorf("Unable to get stat of %s: %s", absPath, err)
// 	}

// 	header := &tar.Header{
// 		Name:    filePath,
// 		Size:    stat.Size(),
// 		Mode:    int64(stat.Mode()),
// 		ModTime: stat.ModTime(),
// 	}

// 	if w.Verbose {
// 		fmt.Fprintf(os.Stderr, "Adding file %s\n", filePath)
// 	}
// 	err = w.Tar.WriteHeader(header)
// 	if err != nil {
// 		return fmt.Errorf("Writing header for file %s failed: %s", absPath, err)
// 	}

// 	_, err = io.Copy(w.Tar, f)
// 	if err != nil {
// 		return fmt.Errorf("Writing file '%s' to tarball failed: %s", absPath, err)
// 	}

// 	return nil
// }

// func (w *tarWriter) addDir(w, dirPath string, absPath string) error {
// 	if w.Verbose {
// 		fmt.Fprintf(os.Stderr, "Adding dir  %s\n", dirPath)
// 	}

// 	dir, err := os.Open(absPath)
// 	if err != nil {
// 		return fmt.Errorf("Could not open directory %s: %s", absPath, err)
// 	}
// 	files, err := dir.Readdir(0)
// 	if err != nil {
// 		return fmt.Errorf("Unable to list files in %s: %s", absPath, err)
// 	}

// 	for _, fileInfo := range files {
// 		if fileInfo.IsDir() {
// 			err = w.addDir(w, filepath.Join(dirPath, fileInfo.Name()), filepath.Join(absPath, fileInfo.Name()))
// 		} else {
// 			err = w.addFile(w, filepath.Join(dirPath, fileInfo.Name()), filepath.Join(absPath, fileInfo.Name()))
// 		}
// 		if err != nil {
// 			return err
// 		}
// 	}

// 	return nil
// }

// func (w *tarWriter) AddEmptyDir(dirPath string) bool {
// 	return true
// }

// func (w *tarWriter) Close() error {
// 	return w.Tar.Close()
// }

// type zipWriter struct {
// 	Zip *zip.ZipArchive
// }

// func newzipWriter(writer io.Writer) *zipWriter {
// 	w := new(zipWriter)
// 	z := zip.New(writer)
// 	w.Zip = z

// 	return w
// }

// func (w *zipWriter) addFile(w, filePath string, absPath string) error {
// 	return w.Zip.addFile(w, filePath, absPath)
// }

// func (w *zipWriter) addDir(w, dirPath string, absPath string) error {
// 	return w.Zip.addDir(w, dirPath, absPath)
// }

// func (w *zipWriter) AddEmptyDir(dirPath string) bool {
// 	return w.Zip.AddEmptyDir(dirPath)
// }

// func (w *zipWriter) Close() error {
// 	return w.Zip.Close()
// }

func addFile(w archiver.Writer, filePath string, absPath string) error {
	file, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}
	err = w.Write(archiver.File{
		FileInfo: archiver.FileInfo{
			FileInfo:   fileInfo,
			CustomName: filePath,
		},
		ReadCloser: file, // does not have to be an actual file
	})

	return nil
}

func addDir(w archiver.Writer, dirPath string, absPath string) error {
	dir, err := os.Open(absPath)
	if err != nil {
		return fmt.Errorf("Could not open directory %s: %s", absPath, err)
	}
	files, err := dir.Readdir(0)
	if err != nil {
		return fmt.Errorf("Unable to list files in %s: %s", absPath, err)
	}

	for _, fileInfo := range files {
		if fileInfo.IsDir() {
			err = addDir(w, filepath.Join(dirPath, fileInfo.Name()), filepath.Join(absPath, fileInfo.Name()))
		} else {
			err = addFile(w, filepath.Join(dirPath, fileInfo.Name()), filepath.Join(absPath, fileInfo.Name()))
		}
		if err != nil {
			return err
		}
	}
	return addFile(w, dirPath, absPath)
}

type outputType struct {
	Enum     []string
	Default  string
	selected string
}

func (e *outputType) Set(value string) error {
	for _, enum := range e.Enum {
		if enum == value {
			e.selected = value
			return nil
		}
	}

	return fmt.Errorf("allowed values are %s", strings.Join(e.Enum, ", "))
}

func (e outputType) String() string {
	if e.selected == "" {
		return e.Default
	}
	return e.selected
}

// CmdDump represents the available dump sub-command.
var CmdDump = cli.Command{
	Name:  "dump",
	Usage: "Dump Gitea files and database",
	Description: `Dump compresses all related files and database into zip file.
It can be used for backup and capture Gitea server image to send to maintainer`,
	Action: runDump,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "file, f",
			Value: fmt.Sprintf("gitea-dump-%d.zip", time.Now().Unix()),
			Usage: "Name of the dump file which will be created. Supply '-' for stdout.",
		},
		cli.BoolFlag{
			Name:  "verbose, V",
			Usage: "Show process details",
		},
		cli.StringFlag{
			Name:  "tempdir, t",
			Value: os.TempDir(),
			Usage: "Temporary dir path",
		},
		cli.StringFlag{
			Name:  "database, d",
			Usage: "Specify the database SQL syntax",
		},
		cli.BoolFlag{
			Name:  "skip-repository, R",
			Usage: "Skip the repository dumping",
		},
		cli.GenericFlag{
			Name: "type",
			Value: &outputType{
				Enum:    []string{"zip", "tar"},
				Default: "zip",
			},
			Usage: "Dump output format. One of zip or tar.",
		},
	},
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	log.Fatal(format, args...)
}

func runDump(ctx *cli.Context) error {
	var file *os.File
	fileName := ctx.String("file")
	if fileName == "-" {
		file = os.Stdout
		log.DelLogger("console")
	}
	setting.NewContext()
	setting.NewServices() // cannot access session settings otherwise

	err := models.SetEngine()
	if err != nil {
		return err
	}

	tmpDir := ctx.String("tempdir")
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		fatal("Path does not exist: %s", tmpDir)
	}
	tmpWorkDir, err := ioutil.TempDir(tmpDir, "gitea-dump-")
	if err != nil {
		fatal("Failed to create tmp work directory: %v", err)
	}
	log.Info("Creating tmp work dir: %s", tmpWorkDir)

	// work-around #1103
	if os.Getenv("TMPDIR") == "" {
		os.Setenv("TMPDIR", tmpWorkDir)
	}

	dbDump := path.Join(tmpWorkDir, "gitea-db.sql")

	log.Info("Packing dump files...")
	if file == nil {
		file, err = os.Create(fileName)
		if err != nil {
			fatal("Unable to open %s: %s", fileName, err)
		}
	}
	defer file.Close()

	// var writer outWriter
	// outType := ctx.String("type")
	// if outType == "tar" {
	// 	writer = newtarWriter(file, ctx.Bool("verbose"))
	// } else if outType == "zip" {
	// 	if fileName != "-" {
	// 		zip.Verbose = ctx.Bool("verbose")
	// 	}
	// 	writer = newzipWriter(file)
	// }
	// defer writer.Close()

	iface, err := archiver.ByExtension(fileName)
	w, _ := iface.(archiver.Writer)
	w.Create(file)
	defer w.Close()

	if ctx.IsSet("skip-repository") {
		log.Info("Skip dumping local repositories")
	} else {
		log.Info("Dumping local repositories...%s", setting.RepoRootPath)
		reposDump := path.Join(tmpWorkDir, "gitea-repo.zip")
		if err := zip.PackTo(setting.RepoRootPath, reposDump, true); err != nil {
			fatal("Failed to dump local repositories: %v", err)
		}
		if err := addFile(w, "gitea-repo.zip", reposDump); err != nil {
			fatal("Failed to include gitea-repo.zip: %v", err)
		}
	}

	targetDBType := ctx.String("database")
	if len(targetDBType) > 0 && targetDBType != setting.Database.Type {
		log.Info("Dumping database %s => %s...", setting.Database.Type, targetDBType)
	} else {
		log.Info("Dumping database...")
	}

	if err := models.DumpDatabase(dbDump, targetDBType); err != nil {
		fatal("Failed to dump database: %v", err)
	}

	if err := addFile(w, "gitea-db.sql", dbDump); err != nil {
		fatal("Failed to include gitea-db.sql: %v", err)
	}

	if len(setting.CustomConf) > 0 {
		log.Info("Adding custom configuration file from %s", setting.CustomConf)
		if err := addFile(w, "app.ini", setting.CustomConf); err != nil {
			fatal("Failed to include specified app.ini: %v", err)
		}
	}

	customDir, err := os.Stat(setting.CustomPath)
	if err == nil && customDir.IsDir() {
		if err := addDir(w, "custom", setting.CustomPath); err != nil {
			fatal("Failed to include custom: %v", err)
		}
	} else {
		log.Info("Custom dir %s doesn't exist, skipped", setting.CustomPath)
	}

	if com.IsExist(setting.AppDataPath) {
		log.Info("Packing data directory...%s", setting.AppDataPath)

		var sessionAbsPath string
		if setting.Cfg.Section("session").Key("PROVIDER").Value() == "file" {
			var opts session.Options
			if err = json.Unmarshal([]byte(setting.SessionConfig.ProviderConfig), &opts); err != nil {
				return err
			}
			sessionAbsPath = opts.ProviderConfig
		}
		if err := addDirExclude(w, "data", setting.AppDataPath, sessionAbsPath); err != nil {
			fatal("Failed to include data directory: %v", err)
		}
	}

	if com.IsExist(setting.LogRootPath) {
		if err := addDir(w, "log", setting.LogRootPath); err != nil {
			fatal("Failed to include log: %v", err)
		}
	}

	if fileName != "-" {
		if err = w.Close(); err != nil {
			_ = os.Remove(fileName)
			fatal("Failed to save %s: %v", fileName, err)
		}

		if err := os.Chmod(fileName, 0600); err != nil {
			log.Info("Can't change file access permissions mask to 0600: %v", err)
		}
	}

	log.Info("Removing tmp work dir: %s", tmpWorkDir)

	if err := os.RemoveAll(tmpWorkDir); err != nil {
		fatal("Failed to remove %s: %v", tmpWorkDir, err)
	}
	log.Info("Finish dumping in file %s", fileName)

	return nil
}

// addDirExclude zips absPath to specified insidePath inside writer excluding excludeAbsPath
func addDirExclude(w archiver.Writer, insidePath, absPath string, excludeAbsPath string) error {
	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return err
	}
	dir, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer dir.Close()

	files, err := dir.Readdir(0)
	if err != nil {
		return err
	}
	for _, file := range files {
		currentAbsPath := path.Join(absPath, file.Name())
		currentInsidePath := path.Join(insidePath, file.Name())
		if file.IsDir() {
			if currentAbsPath != excludeAbsPath {
				if err = addDirExclude(w, currentInsidePath, currentAbsPath, excludeAbsPath); err != nil {
					return err
				}
			}

		} else {
			if err = addFile(w, currentInsidePath, currentAbsPath); err != nil {
				return err
			}
		}
	}
	return nil
}
