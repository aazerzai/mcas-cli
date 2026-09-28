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

var (
	messagesAttachments bool
	messagesOut         string
)

var messagesCmd = &cobra.Command{
	Use:   "messages [message-id] [--attachments [attachment-id...]]",
	Short: "Show messages from teachers and the school, newest first",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if messagesAttachments {
			if len(args) == 0 {
				return fmt.Errorf("--attachments needs a message id")
			}
			return runMessageAttachmentsCmd(cmd, args)
		}
		if cmd.Flags().Changed("out") {
			return fmt.Errorf("-o/--out can only be used with --attachments")
		}
		if len(args) > 1 {
			return fmt.Errorf("unexpected argument %q: pass --attachments to download attachments", args[1])
		}
		if len(args) == 1 {
			return runMessageByID(cmd, args[0])
		}
		return runMessagesList(cmd)
	},
}

func init() {
	messagesCmd.Flags().IntVar(&messagesLimit, "limit", 0, "show at most N messages, applied after filtering (default: all)")
	messagesCmd.Flags().IntVar(&messagesFrom, "from", 0, "only messages from this recipient id")
	messagesCmd.Flags().BoolVar(&messagesUnread, "unread", false, "only unread messages")
	messagesCmd.Flags().StringVar(&messagesSince, "since", "", "only messages on or after this date, YYYY-MM-DD (local time)")
	messagesCmd.Flags().BoolVar(&messagesAttachments, "attachments", false, "download attachments of a message; ids after the message id select specific ones (default: all)")
	messagesCmd.Flags().StringVarP(&messagesOut, "out", "o", "", "with --attachments: directory (or, for one file, file path) to save to (default: current directory)")
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

func rejectListFlags(cmd *cobra.Command) error {
	for _, name := range []string{"limit", "from", "unread", "since"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--%s can't be used with a message id", name)
		}
	}
	return nil
}

// runMessageByID looks up a single message. List flags are rejected here
// (rather than silently ignored) before the id itself is validated, since
// whether a positional argument was given at all - not whether it parses -
// is what puts the command in id mode.
func runMessageByID(cmd *cobra.Command, arg string) error {
	if err := rejectListFlags(cmd); err != nil {
		return err
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

// runMessageAttachmentsCmd validates the positional arguments (message id
// followed by optional attachment ids) and hands off to runMessageAttachments.
func runMessageAttachmentsCmd(cmd *cobra.Command, args []string) error {
	if err := rejectListFlags(cmd); err != nil {
		return err
	}
	messageID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid message id %q: want an integer", args[0])
	}
	var attachmentIDs []int
	seen := map[int]bool{}
	for _, arg := range args[1:] {
		id, err := strconv.Atoi(arg)
		if err != nil {
			return fmt.Errorf("invalid attachment id %q: want an integer", arg)
		}
		if !seen[id] {
			seen[id] = true
			attachmentIDs = append(attachmentIDs, id)
		}
	}
	return runMessageAttachments(messageID, attachmentIDs)
}

// runMessageAttachments downloads the given attachments of a message, or all
// of them when attachmentIDs is empty. Everything is validated before the
// first download, so a typo never leaves a partial result.
func runMessageAttachments(messageID int, attachmentIDs []int) error {
	const command = "messages"
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
		return emitFailure(format, command, output.AuthErrorType, err)
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
	// conversations listing has to be consulted first to name the files.
	conversations, err := loadConversations(c, creds)
	if err != nil {
		return emitFailure(format, command, classifyError(err), err)
	}
	message, err := mcas.FindMessage(conversations, messageID)
	if err != nil {
		return emitFailure(format, command, classifyError(err), err)
	}
	if len(message.Attachments) == 0 {
		return emitFailure(format, command, output.APIErrorType, fmt.Errorf("message %d has no attachments", messageID))
	}

	selected := message.Attachments
	if len(attachmentIDs) > 0 {
		selected = nil
		for _, id := range attachmentIDs {
			found := false
			for _, a := range message.Attachments {
				if a.ID == id {
					selected = append(selected, a)
					found = true
					break
				}
			}
			if !found {
				return emitFailure(format, command, output.APIErrorType, fmt.Errorf("no attachment %d on message %d", id, messageID))
			}
		}
	}

	if len(selected) > 1 && messagesOut != "" {
		if err := prepareOutDir(messagesOut, len(selected)); err != nil {
			return err
		}
	}

	client := mcas.New(creds)
	if err := client.Login(); err != nil {
		return emitFailure(format, command, classifyError(err), err)
	}

	results := make([]attachmentResult, 0, len(selected))
	for _, a := range selected {
		result, err := downloadAttachment(client, messageID, a)
		if err != nil {
			if len(selected) > 1 {
				err = fmt.Errorf("downloaded %d of %d; attachment %d failed: %w", len(results), len(selected), a.ID, err)
			}
			return emitFailure(format, command, output.APIErrorType, err)
		}
		results = append(results, result)
	}
	return output.Result(os.Stdout, format, command, results, renderAttachmentResultsText)
}

// prepareOutDir makes sure -o is usable as a directory for several files:
// an existing directory is kept, a missing path is created, and an existing
// non-directory is an error.
func prepareOutDir(out string, count int) error {
	info, err := os.Stat(out)
	switch {
	case err == nil && !info.IsDir():
		return fmt.Errorf("-o must be a directory when downloading %d attachments", count)
	case err == nil:
		return nil
	case os.IsNotExist(err):
		return os.MkdirAll(out, 0o700)
	default:
		return err
	}
}

// downloadAttachment fetches one attachment and saves it. The destination is
// only claimed once the download has succeeded, so a failed fetch never
// truncates an existing -o file or leaves a 0-byte stub behind.
func downloadAttachment(client *mcas.Client, messageID int, a mcas.MessageAttachment) (attachmentResult, error) {
	data, err := client.MessageAttachmentData(messageID, a.ID)
	if err != nil {
		return attachmentResult{}, err
	}
	destPath, err := resolveAttachmentPath(messagesOut, a.FileName)
	if err != nil {
		return attachmentResult{}, err
	}
	if err := os.WriteFile(destPath, data, 0o600); err != nil {
		return attachmentResult{}, err
	}
	return attachmentResult{MessageID: messageID, AttachmentID: a.ID, FileName: a.FileName, Path: destPath}, nil
}

type attachmentResult struct {
	MessageID    int    `json:"message_id"`
	AttachmentID int    `json:"attachment_id"`
	FileName     string `json:"file_name"`
	Path         string `json:"path"`
}

func renderAttachmentResultsText(w io.Writer, data any) error {
	for _, r := range data.([]attachmentResult) {
		if _, err := fmt.Fprintf(w, "Saved %s to %s\n", r.FileName, r.Path); err != nil {
			return err
		}
	}
	return nil
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
			if _, err := fmt.Fprintf(w, " - [%d] %s   (mcas messages %d --attachments %d)\n", a.ID, a.FileName, m.ID, a.ID); err != nil {
				return err
			}
		}
		if len(m.Attachments) > 1 {
			if _, err := fmt.Fprintf(w, "Download all: mcas messages %d --attachments\n", m.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
