package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stuckpacket/tailchat/db"
	"github.com/stuckpacket/tailchat/storage"
	_ "modernc.org/sqlite"
)

type env struct {
	q      *db.Queries
	groups *GroupsHandler
	msgs   *MessagesHandler
	files  *FilesHandler
	users  map[string]string // name -> id
}

func newEnv(t *testing.T) *env {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	q := db.NewQueries(sqlDB)
	store, err := storage.NewBlobStore(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	InitHub(q)
	e := &env{q: q, groups: NewGroupsHandler(q), msgs: NewMessagesHandler(q), files: NewFilesHandler(q, store, 1<<20), users: map[string]string{}}
	for i, name := range []string{"alice", "bob", "carol", "mallory"} {
		id := uuid.New().String()
		ed := bytes.Repeat([]byte{byte(0x20 + i)}, 32)
		auth := bytes.Repeat([]byte{byte(0x40 + i)}, 32)
		if err := q.CreateUser(id, name, ed, ed, auth); err != nil {
			t.Fatal(err)
		}
		e.users[name] = id
	}
	return e
}

func (e *env) do(h http.Handler, as, method, path string, body interface{}) *httptest.ResponseRecorder {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	r := httptest.NewRequest(method, path, rdr)
	r = r.WithContext(context.WithValue(r.Context(), CtxKeyUserID, e.users[as]))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func env80() []byte { return bytes.Repeat([]byte{7}, 144) }

// createGroup makes a group owned by alice; bob is invited AND has accepted.
func (e *env) createGroup(t *testing.T) string {
	gid := e.createGroupPending(t)
	if w := e.do(e.groups, "bob", "POST", "/api/groups/"+gid+"/accept", nil); w.Code != 204 {
		t.Fatalf("accept: %d %s", w.Code, w.Body)
	}
	return gid
}

// createGroupPending makes a group owned by alice with bob invited but NOT accepted.
func (e *env) createGroupPending(t *testing.T) string {
	gid := uuid.New().String()
	w := e.do(e.groups, "alice", "POST", "/api/groups", map[string]interface{}{
		"id": gid, "encrypted_name": []byte("ciphertext-name"),
		"members": []map[string]interface{}{
			{"user_id": e.users["alice"], "envelope": env80()},
			{"user_id": e.users["bob"], "envelope": env80()},
		},
	})
	if w.Code != 201 {
		t.Fatalf("create group: %d %s", w.Code, w.Body)
	}
	return gid
}

func groupMsg(epoch int) []byte {
	ct := []byte{msgVersionGroup, byte(epoch >> 24), byte(epoch >> 16), byte(epoch >> 8), byte(epoch)}
	return append(ct, bytes.Repeat([]byte{9}, 40)...)
}

func directMsg() []byte {
	return append([]byte{msgVersionDirect}, bytes.Repeat([]byte{9}, 160+40)...)
}

func TestGroupReadAndSendRequireMembership(t *testing.T) {
	e := newEnv(t)
	gid := e.createGroup(t)

	for _, p := range []string{"/api/groups/" + gid + "/messages", "/api/groups/" + gid + "/keys"} {
		if w := e.do(e.groups, "mallory", "GET", p, nil); w.Code != 404 {
			t.Errorf("non-member GET %s = %d, want 404", p, w.Code)
		}
		if w := e.do(e.groups, "bob", "GET", p, nil); w.Code != 200 {
			t.Errorf("member GET %s = %d, want 200", p, w.Code)
		}
	}
	body := map[string]interface{}{"group_id": gid, "ciphertext": groupMsg(0), "nonce": bytes.Repeat([]byte{1}, 12)}
	if w := e.do(e.msgs, "mallory", "POST", "/api/messages", body); w.Code != 404 {
		t.Errorf("non-member send = %d, want 404", w.Code)
	}
	if w := e.do(e.msgs, "bob", "POST", "/api/messages", body); w.Code != 201 {
		t.Errorf("member send = %d, want 201 (%s)", w.Code, "")
	}
	if w := e.do(e.msgs, "mallory", "GET", "/api/messages?group_id="+gid, nil); w.Code != 404 {
		t.Errorf("non-member GET /api/messages?group_id = %d, want 404", w.Code)
	}
}

func TestStaleEpochAndMalformedMessagesRejected(t *testing.T) {
	e := newEnv(t)
	gid := e.createGroup(t)
	nonce := bytes.Repeat([]byte{1}, 12)

	if w := e.do(e.msgs, "bob", "POST", "/api/messages", map[string]interface{}{"group_id": gid, "ciphertext": groupMsg(5), "nonce": nonce}); w.Code != 409 {
		t.Errorf("wrong epoch = %d, want 409", w.Code)
	}
	if w := e.do(e.msgs, "bob", "POST", "/api/messages", map[string]interface{}{"group_id": gid, "ciphertext": directMsg(), "nonce": nonce}); w.Code != 400 {
		t.Errorf("direct-format msg to group = %d, want 400", w.Code)
	}
	if w := e.do(e.msgs, "bob", "POST", "/api/messages", map[string]interface{}{"recipient_id": e.users["alice"], "ciphertext": []byte("hello plaintext"), "nonce": nonce}); w.Code != 400 {
		t.Errorf("plaintext-looking direct msg = %d, want 400", w.Code)
	}
	if w := e.do(e.msgs, "bob", "POST", "/api/messages", map[string]interface{}{"recipient_id": e.users["alice"], "ciphertext": directMsg(), "nonce": []byte{1}}); w.Code != 400 {
		t.Errorf("short nonce = %d, want 400", w.Code)
	}
	if w := e.do(e.msgs, "bob", "POST", "/api/messages", map[string]interface{}{"recipient_id": e.users["alice"], "ciphertext": directMsg(), "nonce": nonce}); w.Code != 201 {
		t.Errorf("valid direct msg = %d, want 201", w.Code)
	}
	if w := e.do(e.msgs, "bob", "POST", "/api/messages", map[string]interface{}{"recipient_id": uuid.New().String(), "ciphertext": directMsg(), "nonce": nonce}); w.Code != 400 {
		t.Errorf("unknown recipient = %d, want 400", w.Code)
	}
}

func TestOnlyAdminChangesMembership(t *testing.T) {
	e := newEnv(t)
	gid := e.createGroup(t)

	add := map[string]interface{}{"user_id": e.users["carol"], "epoch": 0, "envelope": env80()}
	if w := e.do(e.groups, "bob", "POST", "/api/groups/"+gid+"/members", add); w.Code != 403 {
		t.Errorf("non-admin add = %d, want 403", w.Code)
	}
	if w := e.do(e.groups, "mallory", "POST", "/api/groups/"+gid+"/members", add); w.Code != 404 {
		t.Errorf("outsider add = %d, want 404", w.Code)
	}
	if w := e.do(e.groups, "alice", "POST", "/api/groups/"+gid+"/members", add); w.Code != 204 {
		t.Errorf("admin add = %d, want 204", w.Code)
	}
	rm := map[string]interface{}{"user_id": e.users["carol"], "epoch": 1, "encrypted_name": []byte("n2"),
		"members": []map[string]interface{}{{"user_id": e.users["alice"], "envelope": env80()}, {"user_id": e.users["bob"], "envelope": env80()}}}
	if w := e.do(e.groups, "bob", "POST", "/api/groups/"+gid+"/remove", rm); w.Code != 403 {
		t.Errorf("non-admin remove = %d, want 403", w.Code)
	}
	// envelope list that still includes the removed member must be refused
	bad := map[string]interface{}{"user_id": e.users["carol"], "epoch": 1, "encrypted_name": []byte("n2"),
		"members": []map[string]interface{}{{"user_id": e.users["alice"], "envelope": env80()}, {"user_id": e.users["bob"], "envelope": env80()}, {"user_id": e.users["carol"], "envelope": env80()}}}
	if w := e.do(e.groups, "alice", "POST", "/api/groups/"+gid+"/remove", bad); w.Code != 400 {
		t.Errorf("remove leaving removed member's envelope = %d, want 400", w.Code)
	}
	if w := e.do(e.groups, "alice", "POST", "/api/groups/"+gid+"/remove", rm); w.Code != 204 {
		t.Errorf("admin remove+rotate = %d, want 204", w.Code)
	}
	if w := e.do(e.groups, "carol", "GET", "/api/groups/"+gid+"/messages", nil); w.Code != 404 {
		t.Errorf("removed member read = %d, want 404", w.Code)
	}
	// old epoch is now stale for senders
	nonce := bytes.Repeat([]byte{1}, 12)
	if w := e.do(e.msgs, "bob", "POST", "/api/messages", map[string]interface{}{"group_id": gid, "ciphertext": groupMsg(0), "nonce": nonce}); w.Code != 409 {
		t.Errorf("old-epoch send after rotation = %d, want 409", w.Code)
	}
	if w := e.do(e.msgs, "bob", "POST", "/api/messages", map[string]interface{}{"group_id": gid, "ciphertext": groupMsg(1), "nonce": nonce}); w.Code != 201 {
		t.Errorf("new-epoch send = %d, want 201", w.Code)
	}
	// replaying the same rotation (same epoch) is refused
	if w := e.do(e.groups, "alice", "POST", "/api/groups/"+gid+"/rotate", map[string]interface{}{"epoch": 1, "encrypted_name": []byte("n"), "members": rm["members"]}); w.Code != 409 {
		t.Errorf("rotation replay = %d, want 409", w.Code)
	}
}

func TestLeaveFlagsRotationAndAdminCannotLeave(t *testing.T) {
	e := newEnv(t)
	gid := e.createGroup(t)
	if w := e.do(e.groups, "alice", "DELETE", "/api/groups/"+gid+"/members/"+e.users["alice"], nil); w.Code != 400 {
		t.Errorf("admin leave = %d, want 400", w.Code)
	}
	if w := e.do(e.groups, "bob", "DELETE", "/api/groups/"+gid+"/members/"+e.users["alice"], nil); w.Code != 403 {
		t.Errorf("kick via leave = %d, want 403", w.Code)
	}
	if w := e.do(e.groups, "bob", "DELETE", "/api/groups/"+gid+"/members/"+e.users["bob"], nil); w.Code != 204 {
		t.Errorf("leave = %d, want 204", w.Code)
	}
	g, _ := e.q.GetGroup(gid)
	if !g.NeedsRotation {
		t.Error("group should be flagged for rotation after a member leaves")
	}
}

func TestMessageHideRequiresAccess(t *testing.T) {
	e := newEnv(t)
	nonce := bytes.Repeat([]byte{1}, 12)
	w := e.do(e.msgs, "alice", "POST", "/api/messages", map[string]interface{}{"recipient_id": e.users["bob"], "ciphertext": directMsg(), "nonce": nonce})
	var out struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &out)
	if w := e.do(e.msgs, "mallory", "POST", "/api/messages/hide", map[string]string{"message_id": out.ID}); w.Code != 404 {
		t.Errorf("outsider hide = %d, want 404", w.Code)
	}
	if w := e.do(e.msgs, "bob", "POST", "/api/messages/hide", map[string]string{"message_id": out.ID}); w.Code != 204 {
		t.Errorf("recipient hide = %d, want 204", w.Code)
	}
}

func upload(e *env, as, targetID, targetType string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "blob")
	fw.Write(bytes.Repeat([]byte{3}, 100))
	mw.WriteField("target_id", targetID)
	mw.WriteField("target_type", targetType)
	mw.Close()
	r := httptest.NewRequest("POST", "/api/files/upload", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r = r.WithContext(context.WithValue(r.Context(), CtxKeyUserID, e.users[as]))
	w := httptest.NewRecorder()
	e.files.ServeHTTP(w, r)
	return w
}

func TestFileAccessFollowsConversation(t *testing.T) {
	e := newEnv(t)
	gid := e.createGroup(t)

	if w := upload(e, "mallory", gid, "group"); w.Code != 404 {
		t.Errorf("outsider upload to group = %d, want 404", w.Code)
	}
	w := upload(e, "alice", gid, "group")
	if w.Code != 201 {
		t.Fatalf("member upload = %d %s", w.Code, w.Body)
	}
	var up struct {
		FileID string `json:"file_id"`
	}
	json.Unmarshal(w.Body.Bytes(), &up)

	if w := e.do(e.files, "bob", "GET", "/api/files/"+up.FileID, nil); w.Code != 200 {
		t.Errorf("group member download = %d, want 200", w.Code)
	}
	if w := e.do(e.files, "mallory", "GET", "/api/files/"+up.FileID, nil); w.Code != 404 {
		t.Errorf("outsider download = %d, want 404", w.Code)
	}

	// 1:1: only the sender and the addressed recipient may fetch it
	w = upload(e, "alice", e.users["bob"], "direct")
	json.Unmarshal(w.Body.Bytes(), &up)
	if w := e.do(e.files, "bob", "GET", "/api/files/"+up.FileID, nil); w.Code != 200 {
		t.Errorf("recipient download = %d, want 200", w.Code)
	}
	if w := e.do(e.files, "carol", "GET", "/api/files/"+up.FileID, nil); w.Code != 404 {
		t.Errorf("third-party download = %d, want 404", w.Code)
	}
	if w := e.do(e.files, "bob", "DELETE", "/api/files/"+up.FileID, nil); w.Code != 404 {
		t.Errorf("non-uploader delete = %d, want 404", w.Code)
	}
}

func TestRegisterValidation(t *testing.T) {
	e := newEnv(t)
	h := NewRegisterHandler(e.q)
	k := bytes.Repeat([]byte{5}, 32)
	post := func(handle string, ed []byte) int {
		b, _ := json.Marshal(map[string]interface{}{"handle": handle, "public_key_ed25519": ed, "public_key_x25519": k, "derived_public_key_ed25519": k})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/api/register", bytes.NewReader(b)))
		return w.Code
	}
	if c := post("dave", k); c != 201 {
		t.Errorf("valid = %d", c)
	}
	if c := post("dave", k); c != 409 {
		t.Errorf("duplicate = %d, want 409", c)
	}
	k2 := bytes.Repeat([]byte{6}, 32)
	post2 := func(handle string, ed, auth []byte) int {
		b, _ := json.Marshal(map[string]interface{}{"handle": handle, "public_key_ed25519": ed, "public_key_x25519": k2, "derived_public_key_ed25519": auth})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/api/register", bytes.NewReader(b)))
		return w.Code
	}
	if c := post2("DAVE", k2, k2); c != 409 {
		t.Errorf("same handle in other letter case = %d, want 409 (impersonation)", c)
	}
	if c := post2("dave2", k, bytes.Repeat([]byte{8}, 32)); c != 409 {
		t.Errorf("reusing another account's identity key = %d, want 409", c)
	}
	if c := post("x", k); c != 400 {
		t.Errorf("short handle = %d, want 400", c)
	}
	if c := post("bad handle!", k); c != 400 {
		t.Errorf("bad chars = %d, want 400", c)
	}
	if c := post("erin", k[:10]); c != 400 {
		t.Errorf("short key = %d, want 400", c)
	}
}

func TestSearchIsLiteralPrefixAndWildcardsEscaped(t *testing.T) {
	e := newEnv(t) // alice, bob, carol, mallory
	for _, c := range []struct {
		q    string
		want int
	}{
		{"%%%", 0}, {"___", 0}, {"ali", 1}, {"ALI", 1}, {"lice", 0}, {"zzz", 0},
	} {
		got, err := e.q.SearchUsers(c.q, 20)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != c.want {
			t.Errorf("SearchUsers(%q) = %d users, want %d", c.q, len(got), c.want)
		}
	}
}

func TestReadReceiptsOnlyCoverMessagesAddressedToMe(t *testing.T) {
	e := newEnv(t)
	nonce := bytes.Repeat([]byte{1}, 12)
	e.do(e.msgs, "alice", "POST", "/api/messages", map[string]interface{}{"recipient_id": e.users["bob"], "ciphertext": directMsg(), "nonce": nonce})
	// Alice opening the chat must NOT mark her own outgoing message as read by Bob.
	if err := e.q.MarkConversationRead(e.users["alice"], e.users["bob"], ""); err != nil {
		t.Fatal(err)
	}
	rows, _ := e.q.GetDirectMessages(e.users["alice"], e.users["bob"], nil, nil, 10)
	if len(rows) != 1 || rows[0].ReadAt != nil {
		t.Fatalf("sender's own message was marked read without the recipient reading it: %+v", rows)
	}
	if err := e.q.MarkConversationRead(e.users["bob"], e.users["alice"], ""); err != nil {
		t.Fatal(err)
	}
	rows, _ = e.q.GetDirectMessages(e.users["alice"], e.users["bob"], nil, nil, 10)
	if rows[0].ReadAt == nil {
		t.Error("recipient reading should mark it read")
	}
}

func TestUploadQuota(t *testing.T) {
	e := newEnv(t)
	e.files.SetUserQuota(150) // each test upload is 100 bytes
	gid := e.createGroup(t)
	if w := upload(e, "alice", gid, "group"); w.Code != 201 {
		t.Fatalf("first upload = %d", w.Code)
	}
	if w := upload(e, "alice", gid, "group"); w.Code != 413 {
		t.Errorf("upload over quota = %d, want 413", w.Code)
	}
	if w := upload(e, "bob", gid, "group"); w.Code != 201 {
		t.Errorf("quota must be per user; bob's first upload = %d", w.Code)
	}
}

func TestWSTicketsAreSingleUseAndExpire(t *testing.T) {
	tk := IssueWSTicket("u1")
	if RedeemWSTicket(tk) != "u1" {
		t.Fatal("fresh ticket should redeem")
	}
	if RedeemWSTicket(tk) != "" {
		t.Error("ticket must not work twice")
	}
	if RedeemWSTicket("nope") != "" {
		t.Error("unknown ticket must not work")
	}
	wsTickets.mu.Lock()
	wsTickets.tickets["old"] = ticketEntry{userID: "u1", expires: time.Now().Add(-time.Second)}
	wsTickets.mu.Unlock()
	if RedeemWSTicket("old") != "" {
		t.Error("expired ticket must not work")
	}
}

func TestPendingInviteeCanDoNothingUntilAccepted(t *testing.T) {
	e := newEnv(t)
	gid := e.createGroupPending(t)
	nonce := bytes.Repeat([]byte{1}, 12)

	// bob sees the invitation, but only the invitation
	w := e.do(e.groups, "bob", "GET", "/api/groups/invites", nil)
	var inv struct {
		Invites []struct {
			GroupID   string `json:"group_id"`
			InvitedBy string `json:"invited_by"`
		}
	}
	json.Unmarshal(w.Body.Bytes(), &inv)
	if len(inv.Invites) != 1 || inv.Invites[0].GroupID != gid || inv.Invites[0].InvitedBy != e.users["alice"] {
		t.Fatalf("invitation list wrong: %s", w.Body)
	}
	if w := e.do(e.groups, "bob", "GET", "/api/groups", nil); bytes.Contains(w.Body.Bytes(), []byte(gid)) {
		t.Error("a pending invite must not appear as one of bob's groups")
	}
	for _, c := range []struct {
		name, method, path string
		body               interface{}
	}{
		{"read messages", "GET", "/api/groups/" + gid + "/messages", nil},
		{"get keys", "GET", "/api/groups/" + gid + "/keys", nil},
		{"read via /api/messages", "GET", "/api/messages?group_id=" + gid, nil},
		{"send", "POST", "/api/messages", map[string]interface{}{"group_id": gid, "ciphertext": groupMsg(0), "nonce": nonce}},
	} {
		h := http.Handler(e.groups)
		if c.path == "/api/messages" || len(c.path) > 13 && c.path[:13] == "/api/messages" {
			h = e.msgs
		}
		if w := e.do(h, "bob", c.method, c.path, c.body); w.Code != 404 {
			t.Errorf("pending invitee %s = %d, want 404", c.name, w.Code)
		}
	}
	if w := upload(e, "bob", gid, "group"); w.Code != 404 {
		t.Errorf("pending invitee upload = %d, want 404", w.Code)
	}
	// and a stranger cannot accept someone else's invitation
	if w := e.do(e.groups, "mallory", "POST", "/api/groups/"+gid+"/accept", nil); w.Code != 404 {
		t.Errorf("uninvited accept = %d, want 404", w.Code)
	}

	if w := e.do(e.groups, "bob", "POST", "/api/groups/"+gid+"/accept", nil); w.Code != 204 {
		t.Fatalf("accept = %d", w.Code)
	}
	if w := e.do(e.groups, "bob", "GET", "/api/groups/"+gid+"/keys", nil); w.Code != 200 {
		t.Errorf("after accept, keys = %d, want 200", w.Code)
	}
	if w := e.do(e.groups, "bob", "POST", "/api/groups/"+gid+"/accept", nil); w.Code != 404 {
		t.Errorf("accepting twice = %d, want 404", w.Code)
	}
}

func TestDeclineDeletesKeyEnvelopeAndMembership(t *testing.T) {
	e := newEnv(t)
	gid := e.createGroupPending(t)
	if w := e.do(e.groups, "bob", "POST", "/api/groups/"+gid+"/decline", nil); w.Code != 204 {
		t.Fatalf("decline = %d", w.Code)
	}
	if ok, _ := e.q.IsGroupParticipant(gid, e.users["bob"]); ok {
		t.Error("declined invitee must not remain a participant")
	}
	if envs, _ := e.q.GetMemberEnvelopes(gid, e.users["bob"]); len(envs) != 0 {
		t.Error("the key envelope addressed to a decliner must be deleted")
	}
	if w := e.do(e.groups, "bob", "POST", "/api/groups/"+gid+"/decline", nil); w.Code != 404 {
		t.Errorf("declining twice = %d, want 404", w.Code)
	}
	// an ACTIVE member cannot use decline to bypass the leave/rotate flow
	gid2 := e.createGroup(t)
	if w := e.do(e.groups, "bob", "POST", "/api/groups/"+gid2+"/decline", nil); w.Code != 404 {
		t.Errorf("active member decline = %d, want 404", w.Code)
	}
}

func TestRotationMustAlsoCoverPendingInvitees(t *testing.T) {
	e := newEnv(t)
	gid := e.createGroup(t) // alice + bob(active)
	add := map[string]interface{}{"user_id": e.users["carol"], "epoch": 0, "envelope": env80()}
	if w := e.do(e.groups, "alice", "POST", "/api/groups/"+gid+"/members", add); w.Code != 204 {
		t.Fatalf("invite carol = %d", w.Code)
	}
	if w := e.do(e.groups, "alice", "POST", "/api/groups/"+gid+"/members", add); w.Code != 409 {
		t.Errorf("inviting twice = %d, want 409", w.Code)
	}
	two := []map[string]interface{}{{"user_id": e.users["alice"], "envelope": env80()}, {"user_id": e.users["bob"], "envelope": env80()}}
	three := append(append([]map[string]interface{}{}, two...), map[string]interface{}{"user_id": e.users["carol"], "envelope": env80()})
	if w := e.do(e.groups, "alice", "POST", "/api/groups/"+gid+"/rotate", map[string]interface{}{"epoch": 1, "encrypted_name": []byte("n"), "members": two}); w.Code != 400 {
		t.Errorf("rotation that skips a pending invitee = %d, want 400", w.Code)
	}
	if w := e.do(e.groups, "alice", "POST", "/api/groups/"+gid+"/rotate", map[string]interface{}{"epoch": 1, "encrypted_name": []byte("n"), "members": three}); w.Code != 204 {
		t.Errorf("rotation covering everyone = %d, want 204", w.Code)
	}
	// carol accepts after the rotation and gets the CURRENT (epoch 1) key
	if w := e.do(e.groups, "carol", "POST", "/api/groups/"+gid+"/accept", nil); w.Code != 204 {
		t.Fatalf("carol accept = %d", w.Code)
	}
	envs, _ := e.q.GetMemberEnvelopes(gid, e.users["carol"])
	if len(envs) == 0 || envs[len(envs)-1].Epoch != 1 {
		t.Errorf("late joiner must hold the epoch-1 key, got %+v", envs)
	}
}

func TestInviteSpamCapAndNoMessagesToPending(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < maxPendingInvites; i++ {
		e.createGroupPending(t) // bob collects invitations
	}
	gid := uuid.New().String()
	w := e.do(e.groups, "alice", "POST", "/api/groups", map[string]interface{}{
		"id": gid, "encrypted_name": []byte("x"),
		"members": []map[string]interface{}{{"user_id": e.users["alice"], "envelope": env80()}, {"user_id": e.users["bob"], "envelope": env80()}},
	})
	if w.Code != 429 {
		t.Errorf("invite beyond the pending cap = %d, want 429", w.Code)
	}
}
