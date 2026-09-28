package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/dental-dash/my-child-at-school-cli/internal/cache"
	"github.com/dental-dash/my-child-at-school-cli/internal/config"
	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
	"github.com/dental-dash/my-child-at-school-cli/internal/output"
)

var (
	messagesLimit  int
	messagesFrom   int
	messagesUnread bool
	messagesSince  string
)

var messagesCmd = &cobra.Command{
	Use:   "messages [message-id]",
	Short: "Show messages from teachers and the school, newest first",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return runMessageByID(cmd, args[0])
		}
		return runMessagesList(cmd)
	},
}

var messagesAttachmentOut string

var messagesAttachmentCmd = &cobra.Command{
	Use:   "attachment <message-id> <attachment-id>",
	Short: "Download a message attachment",
	Args:  cobra.ExactArgs(2),
	RunE:  runMessagesAttachment,
}

func init() {
	messagesCmd.Flags().IntVar(&messagesLimit, "limit", 0, "show at most N messages, applied after filtering (default: all)")
	messagesCmd.Flags().IntVar(&messagesFrom, "from", 0, "only messages from this recipient id")
	messagesCmd.Flags().BoolVar(&messagesUnread, "unread", false, "only unread messages")
	messagesCmd.Flags().StringVar(&messagesSince, "since", "", "only messages on or after this date, YYYY-MM-DD (local time)")
	messagesAttachmentCmd.Flags().StringVarP(&messagesAttachmentOut, "out", "o", "", "file or directory to save the attachment to (default: current directory)")
	messagesCmd.AddCommand(messagesAttachmentCmd)
	rootCmd.AddCommand(messagesCmd)
}

// loadConversations serves the shared "all" cache entry that both list mode,
// message-id mode and messages attachment read, logging in and fetching
// fresh from MCAS on a cache miss.
func loadConversations(c *cache.Cache, creds mcas.Credentials) ([]mcas.Conversation, error) {
	return fetchCached(c, "messages:"+creds.Email+":all", forceRefresh, func() ([]mcas.Conversation, error) {
		client := mcas.New(creds)
		if err := client.Login(); err != nil {
			var zero []mcas.Conversation
			return zero, err
		}
		return client.Conversations()
	})
}

// parseSinceFlag parses a --since flag value using the same YYYY-MM-DD
// layout as attendance --date. An empty value means no lower bound.
func parseSinceFlag(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	t, err := time.Parse(dateFormat, value)
	if err != nil {
		return nil, fmt.Errorf("invalid --since %q: want YYYY-MM-DD", value)
	}
	return &t, nil
}

// filterMessages applies --from, --unread and --since, then --limit, to an
// already newest-first message list.
func filterMessages(messages []mcas.InboxMessage, hasFrom bool, since *time.Time) []mcas.InboxMessage {
	filtered := make([]mcas.InboxMessage, 0, len(messages))
	for _, m := range messages {
		if hasFrom && m.RecipientID != messagesFrom {
			continue
		}
		if messagesUnread && m.Read {
			continue
		}
		if since != nil && m.Date.Before(*since) {
			continue
		}
		filtered = append(filtered, m)
	}
	if messagesLimit > 0 && len(filtered) > messagesLimit {
		filtered = filtered[:messagesLimit]
	}
	return filtered
}

func runMessagesList(cmd *cobra.Command) error {
	if messagesLimit < 0 {
		return fmt.Errorf("invalid --limit %d: must be 0 or more", messagesLimit)
	}
	since, err := parseSinceFlag(messagesSince)
	if err != nil {
		return err
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
		return emitFailure(format, "messages", output.AuthErrorType, err)
	}
	cacheDir, err := cfg.CacheDir()
	if err != nil {
		return err
	}
	c, err := cache.New(cacheDir, cfg.CacheTTL())
	if err != nil {
		return err
	}

	conversations, err := loadConversations(c, creds)
	if err != nil {
		return emitFailure(format, "messages", classifyError(err), err)
	}

	messages := filterMessages(mcas.FlattenConversations(conversations), cmd.Flags().Changed("from"), since)
	return output.Result(os.Stdout, format, "messages", messages, renderInboxMessagesText)
}

// runMessageByID looks up a single message. List flags are rejected here
// (rather than silently ignored) before the id itself is validated, since
// whether a positional argument was given at all - not whether it parses -
// is what puts the command in id mode.
func runMessageByID(cmd *cobra.Command, arg string) error {
	for _, name := range []string{"limit", "from", "unread", "since"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--%s can't be used with a message id", name)
		}
	}
	messageID, err := strconv.Atoi(arg)
	if err != nil {
		return fmt.Errorf("invalid message id %q: want an integer", arg)
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
		return emitFailure(format, "messages", output.AuthErrorType, err)
	}
	cacheDir, err := cfg.CacheDir()
	if err != nil {
		return err
	}
	c, err := cache.New(cacheDir, cfg.CacheTTL())
	if err != nil {
		return err
	}

	conversations, err := loadConversations(c, creds)
	if err != nil {
		return emitFailure(format, "messages", classifyError(err), err)
	}

	message, err := mcas.FindMessage(conversations, messageID)
	if err != nil {
		return emitFailure(format, "messages", classifyError(err), err)
	}
	return output.Result(os.Stdout, format, "messages", message, renderInboxMessageText)
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
	conversations, err := loadConversations(c, creds)
	if err != nil {
		return emitFailure(format, "messages attachment", classifyError(err), err)
	}

	fileName := findAttachmentFileName(conversations, messageID, attachmentID)
	if fileName == "" {
		err := fmt.Errorf("no attachment %d on message %d", attachmentID, messageID)
		return emitFailure(format, "messages attachment", output.APIErrorType, err)
	}

	client := mcas.New(creds)
	if err := client.Login(); err != nil {
		return emitFailure(format, "messages attachment", classifyError(err), err)
	}
	data, err := client.MessageAttachmentData(messageID, attachmentID)
	if err != nil {
		return emitFailure(format, "messages attachment", classifyError(err), err)
	}

	// Only claim the destination path once the download has actually
	// succeeded, so a failed login/fetch never truncates an existing -o file
	// or leaves a 0-byte stub behind.
	destPath, err := resolveAttachmentPath(messagesAttachmentOut, fileName)
	if err != nil {
		return err
	}
	if err := os.WriteFile(destPath, data, 0o600); err != nil {
		return err
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

// resolveAttachmentPath decides where an attachment should be written and
// claims that path by creating the (empty) file, so that the name it returns
// is guaranteed not to collide with anything else on disk. fileName is the
// name MCAS reports for the attachment; out is the raw --out flag value.
//
//   - out == "": save `sanitizeFileName(fileName)` in the current directory,
//     de-duplicating against any existing file the way a browser does.
//   - out names an existing directory: same, but inside that directory.
//   - otherwise out is an exact file path, created (and its parent
//     directories) as needed, always overwriting whatever is there.
func resolveAttachmentPath(out, fileName string) (string, error) {
	sanitized := sanitizeFileName(fileName)
	if out == "" {
		return claimUniquePath(".", sanitized)
	}
	if info, err := os.Stat(out); err == nil && info.IsDir() {
		return claimUniquePath(out, sanitized)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		return "", err
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	f.Close()
	return filepath.Abs(out)
}

// claimUniquePath finds the first free "name.ext", "name (1).ext",
// "name (2).ext", ... in dir and atomically claims it by creating it with
// O_EXCL, so a concurrent caller can't win the same name between the check
// and the write.
func claimUniquePath(dir, name string) (string, error) {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 0; ; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", base, i, ext)
		}
		path := filepath.Join(dir, candidate)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			f.Close()
			return filepath.Abs(path)
		}
		if !os.IsExist(err) {
			return "", err
		}
	}
}

// renderInboxMessagesText renders one row per message, newest first, in the
// flat "ID  DATE  FROM  SUBJECT  [N att] [unread]" form.
func renderInboxMessagesText(w io.Writer, data any) error {
	messages := data.([]mcas.InboxMessage)
	if len(messages) == 0 {
		_, err := fmt.Fprintln(w, "No messages.")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ID\tDATE\tFROM\tSUBJECT"); err != nil {
		return err
	}
	for _, m := range messages {
		name := m.RecipientName
		if name == "" {
			name = "(unknown sender)"
		}
		subject := m.Subject
		if len(m.Attachments) > 0 {
			subject += fmt.Sprintf("  [%d att]", len(m.Attachments))
		}
		if !m.Read {
			subject += "  [unread]"
		}
		if _, err := fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", m.ID, m.Date.Format(dateFormat), name, subject); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func renderInboxMessageText(w io.Writer, data any) error {
	m := data.(*mcas.InboxMessage)
	name := m.RecipientName
	if name == "" {
		name = "(unknown sender)"
	}
	direction := "Received"
	if m.Sent {
		direction = "Sent"
	}
	read := "no"
	if m.Read {
		read = "yes"
	}
	if _, err := fmt.Fprintf(w, "From:      %s (recipient id %d)\n", name, m.RecipientID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Date:      %s\n", m.Date.Format("2006-01-02 15:04")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Direction: %s\n", direction); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Read:      %s\n", read); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Subject:   %s\n\n", m.Subject); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, m.Body); err != nil {
		return err
	}
	if len(m.Links) > 0 {
		if _, err := fmt.Fprintln(w, "\nLinks:"); err != nil {
			return err
		}
		for _, l := range m.Links {
			if _, err := fmt.Fprintln(w, " -", l); err != nil {
				return err
			}
		}
	}
	if len(m.Attachments) > 0 {
		if _, err := fmt.Fprintln(w, "\nAttachments:"); err != nil {
			return err
		}
		for _, a := range m.Attachments {
			if _, err := fmt.Fprintf(w, " - [%d] %s   (mcas messages attachment %d %d)\n", a.ID, a.FileName, m.ID, a.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
