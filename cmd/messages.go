package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dental-dash/my-child-at-school-cli/internal/cache"
	"github.com/dental-dash/my-child-at-school-cli/internal/config"
	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
	"github.com/dental-dash/my-child-at-school-cli/internal/output"
)

var messagesCmd = &cobra.Command{
	Use:   "messages",
	Short: "Show messages from teachers and the school",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCommand("messages", "all", func(client *mcas.Client) ([]mcas.Conversation, error) {
			return client.Conversations()
		}, renderConversationsText)
	},
}

var messagesShowCmd = &cobra.Command{
	Use:   "show <recipient-id>",
	Short: "Show the full message thread with one sender",
	Args:  cobra.ExactArgs(1),
	RunE:  runMessagesShow,
}

var messagesAttachmentOut string

var messagesAttachmentCmd = &cobra.Command{
	Use:   "attachment <message-id> <attachment-id>",
	Short: "Download a message attachment",
	Args:  cobra.ExactArgs(2),
	RunE:  runMessagesAttachment,
}

func init() {
	messagesAttachmentCmd.Flags().StringVarP(&messagesAttachmentOut, "out", "o", "", "file path to save the attachment to (default: under the cache directory)")
	messagesCmd.AddCommand(messagesShowCmd)
	messagesCmd.AddCommand(messagesAttachmentCmd)
	rootCmd.AddCommand(messagesCmd)
}

func runMessagesShow(cmd *cobra.Command, args []string) error {
	recipientID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid recipient id %q: want an integer", args[0])
	}
	return runCommand("messages", "show:"+args[0], func(client *mcas.Client) (*mcas.Conversation, error) {
		return client.Conversation(recipientID)
	}, renderConversationText)
}

func runMessagesAttachment(cmd *cobra.Command, args []string) error {
	messageID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid message id %q: want an integer", args[0])
	}
	attachmentID, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid attachment id %q: want an integer", args[1])
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	format, err := resolveFormat(cfg)
	if err != nil {
		return err
	}
	creds, err := cfg.ResolveCredentials(flagEmail, flagPassword)
	if err != nil {
		return emitFailure(format, "messages attachment", output.AuthErrorType, err)
	}

	cacheDir, err := cfg.CacheDir()
	if err != nil {
		return err
	}
	c, err := cache.New(cacheDir, cfg.CacheTTL())
	if err != nil {
		return err
	}

	// The attachment download call itself reports no filename, so the
	// conversations listing has to be consulted first to name the file.
	conversations, err := fetchCached(c, "messages:"+creds.Email+":all", forceRefresh, func() ([]mcas.Conversation, error) {
		client := mcas.New(creds)
		if err := client.Login(); err != nil {
			var zero []mcas.Conversation
			return zero, err
		}
		return client.Conversations()
	})
	if err != nil {
		return emitFailure(format, "messages attachment", classifyError(err), err)
	}

	fileName := findAttachmentFileName(conversations, messageID, attachmentID)
	if fileName == "" {
		err := fmt.Errorf("no attachment %d on message %d", attachmentID, messageID)
		return emitFailure(format, "messages attachment", output.APIErrorType, err)
	}

	destPath := messagesAttachmentOut
	if destPath == "" {
		destPath = filepath.Join(cacheDir, "attachments", strconv.Itoa(messageID), fmt.Sprintf("%d-%s", attachmentID, sanitizeFileName(fileName)))
	}

	// A sent attachment can't change after the fact, so once it's on disk at
	// its deterministic auto-generated path, only an explicit --force-refresh
	// re-fetches it. That immutability rationale doesn't hold for a
	// user-supplied -o path, which should always be (over)written.
	if forceRefresh || messagesAttachmentOut != "" || !fileExists(destPath) {
		client := mcas.New(creds)
		if err := client.Login(); err != nil {
			return emitFailure(format, "messages attachment", classifyError(err), err)
		}
		data, err := client.MessageAttachmentData(messageID, attachmentID)
		if err != nil {
			return emitFailure(format, "messages attachment", classifyError(err), err)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(destPath, data, 0o600); err != nil {
			return err
		}
	}

	result := attachmentResult{Path: destPath, FileName: fileName}
	return output.Result(os.Stdout, format, "messages attachment", result, renderAttachmentResultText)
}

type attachmentResult struct {
	Path     string `json:"path"`
	FileName string `json:"file_name"`
}

func renderAttachmentResultText(w io.Writer, data any) error {
	r := data.(attachmentResult)
	_, err := fmt.Fprintf(w, "Saved %s to %s\n", r.FileName, r.Path)
	return err
}

func findAttachmentFileName(conversations []mcas.Conversation, messageID, attachmentID int) string {
	for _, conv := range conversations {
		for _, m := range conv.Messages {
			if m.ID != messageID {
				continue
			}
			for _, a := range m.Attachments {
				if a.ID == attachmentID {
					return a.FileName
				}
			}
		}
	}
	return ""
}

// sanitizeFileName strips path separators and control characters from a
// MCAS-reported attachment name before it's used as part of a local path.
// filepath.Base is not enough here: it only understands the host OS's own
// separator, but a name could carry either "/" or "\".
func sanitizeFileName(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' })
	base := ""
	if len(parts) > 0 {
		base = parts[len(parts)-1]
	}
	var sb strings.Builder
	for _, r := range base {
		if r >= 0x20 {
			sb.WriteRune(r)
		}
	}
	cleaned := strings.TrimSpace(sb.String())
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return "attachment"
	}
	return cleaned
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func renderConversationsText(w io.Writer, data any) error {
	conversations := data.([]mcas.Conversation)
	if len(conversations) == 0 {
		_, err := fmt.Fprintln(w, "No messages.")
		return err
	}
	for _, conv := range conversations {
		name := conv.RecipientName
		if name == "" {
			name = "(unknown sender)"
		}
		last := "no messages"
		if len(conv.Messages) > 0 {
			m := conv.Messages[len(conv.Messages)-1]
			last = m.Date.Format(dateFormat) + " " + m.Subject
		}
		unread := ""
		if conv.UnreadCount > 0 {
			unread = fmt.Sprintf(" (%d unread)", conv.UnreadCount)
		}
		if _, err := fmt.Fprintf(w, "%d  %s  %d messages%s - %s\n", conv.RecipientID, name, len(conv.Messages), unread, last); err != nil {
			return err
		}
	}
	return nil
}

func renderConversationText(w io.Writer, data any) error {
	conv := data.(*mcas.Conversation)
	name := conv.RecipientName
	if name == "" {
		name = "(unknown sender)"
	}
	if _, err := fmt.Fprintf(w, "%s (recipient id %d)\n", name, conv.RecipientID); err != nil {
		return err
	}
	if len(conv.Messages) == 0 {
		_, err := fmt.Fprintln(w, "No messages in this thread.")
		return err
	}
	for _, m := range conv.Messages {
		direction := "Received"
		if m.Sent {
			direction = "Sent"
		}
		read := ""
		if !m.Read {
			read = " (unread)"
		}
		if _, err := fmt.Fprintf(w, "\n[%s] %s%s - %s\n", m.Date.Format("2006-01-02 15:04"), direction, read, m.Subject); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, m.Body); err != nil {
			return err
		}
		if len(m.Links) > 0 {
			if _, err := fmt.Fprintln(w, "Links:"); err != nil {
				return err
			}
			for _, l := range m.Links {
				if _, err := fmt.Fprintln(w, " -", l); err != nil {
					return err
				}
			}
		}
		if len(m.Attachments) > 0 {
			if _, err := fmt.Fprintln(w, "Attachments:"); err != nil {
				return err
			}
			for _, a := range m.Attachments {
				if _, err := fmt.Fprintf(w, " - [%d] %s\n", a.ID, a.FileName); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
