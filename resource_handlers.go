package tellus

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"

	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/internal/i18n"
	"github.com/TechnoVizor/tellus/internal/secure"
	"github.com/TechnoVizor/tellus/internal/ui"
)

const (
	maxPage        = 1_000_000
	maxSearchRunes = 200
)

func (rs *resource[T]) mount(mux *http.ServeMux) {
	base := "/" + rs.slug
	authed := rs.panel.authed
	mux.HandleFunc("GET "+base, authed(rs.list))
	mux.HandleFunc("GET "+base+"/new", authed(rs.newForm))
	mux.HandleFunc("POST "+base, authed(rs.create))
	mux.HandleFunc("GET "+base+"/{id}/edit", authed(rs.editForm))
	mux.HandleFunc("POST "+base+"/{id}", authed(rs.update))
	mux.HandleFunc("POST "+base+"/{id}/delete", authed(rs.delete))
}

// --- list ---

type listParams struct {
	Search string
	Sort   string // a sortable field name or ""
	Dir    string // "asc" or "desc" when Sort is set
	Page   int
}

// parseList reads the list query string. Anything unexpected falls back to a
// safe default, so a hand-edited URL never breaks the page.
func (rs *resource[T]) parseList(r *http.Request) listParams {
	q := r.URL.Query()
	// CleanText: Postgres rejects NUL bytes and invalid UTF-8 in a query.
	lp := listParams{Search: truncateRunes(strings.TrimSpace(secure.CleanText(q.Get("q"))), maxSearchRunes), Page: 1}
	if n, err := strconv.Atoi(q.Get("page")); err == nil && n > 1 {
		lp.Page = min(n, maxPage)
	}
	if s := q.Get("sort"); rs.sortable[s] { // only declared sortable columns
		lp.Sort, lp.Dir = s, "asc"
		if q.Get("dir") == "desc" {
			lp.Dir = "desc"
		}
	}
	return lp
}

func (rs *resource[T]) listURL(lp listParams) string {
	v := url.Values{}
	if lp.Search != "" {
		v.Set("q", lp.Search)
	}
	if lp.Sort != "" {
		v.Set("sort", lp.Sort)
		v.Set("dir", lp.Dir)
	}
	if lp.Page > 1 {
		v.Set("page", strconv.Itoa(lp.Page))
	}
	u := rs.panel.url("/" + rs.slug)
	if enc := v.Encode(); enc != "" {
		u += "?" + enc
	}
	return u
}

func (rs *resource[T]) list(w http.ResponseWriter, r *http.Request, u User) {
	lp := rs.parseList(r)
	query := ListQuery{
		Search:       lp.Search,
		SearchFields: rs.searchFields,
		SortField:    lp.Sort,
		SortDesc:     lp.Dir == "desc",
		Page:         lp.Page,
		PerPage:      rs.perPage,
	}
	res, err := rs.source.List(r.Context(), query)
	if err != nil {
		rs.panel.serverError(w, err)
		return
	}
	totalPages := max(1, int((res.Total+int64(rs.perPage)-1)/int64(rs.perPage)))
	if lp.Page > totalPages { // page past the end, for example after deleting the last row of a page
		lp.Page, query.Page = totalPages, totalPages
		if res, err = rs.source.List(r.Context(), query); err != nil {
			rs.panel.serverError(w, err)
			return
		}
	}

	view := rs.listView(r, u, lp, res, totalPages)
	w.Header().Add("Vary", "HX-Request")
	// htmx asks for the region only. A history restore needs the whole page.
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-History-Restore-Request") != "true" {
		rs.panel.render(w, r, http.StatusOK, ui.Records(view))
		return
	}
	rs.panel.render(w, r, http.StatusOK, ui.ListPage(view))
}

func (rs *resource[T]) listView(r *http.Request, u User, lp listParams, res ListResult[T], totalPages int) ui.ListView {
	cols := make([]ui.ColumnView, len(rs.columns))
	for i, c := range rs.columns {
		info := c.Info()
		cv := ui.ColumnView{Label: info.Label, Sortable: info.Sortable}
		if info.Sortable {
			next := listParams{Search: lp.Search, Sort: info.Name, Dir: "asc"}
			if lp.Sort == info.Name {
				if lp.Dir == "asc" {
					next.Dir, cv.SortMark, cv.AriaSort = "desc", "▲", "ascending"
				} else {
					cv.SortMark, cv.AriaSort = "▼", "descending"
				}
			}
			cv.SortURL = rs.listURL(next)
		}
		cols[i] = cv
	}

	rows := make([]ui.RowView, len(res.Items))
	for i := range res.Items {
		item := &res.Items[i]
		rv := reflect.ValueOf(item).Elem()
		cells := make([]templ.Component, len(rs.columns))
		for j, c := range rs.columns {
			cells[j] = c.Cell(rv.FieldByIndex(rs.colIndex[j]).Interface())
		}
		id := url.PathEscape(rs.source.ID(item))
		rows[i] = ui.RowView{
			Cells:     cells,
			EditURL:   rs.panel.url("/" + rs.slug + "/" + id + "/edit"),
			DeleteURL: rs.panel.url("/" + rs.slug + "/" + id + "/delete"),
		}
	}

	view := ui.ListView{
		Shell:      rs.panel.shell(r, u, rs.plural, rs.slug),
		Heading:    rs.plural,
		Notice:     rs.notice(r.URL.Query().Get("notice")),
		NewURL:     rs.panel.url("/" + rs.slug + "/new"),
		Search:     lp.Search,
		Searchable: len(rs.searchFields) > 0,
		ListURL:    rs.listURL(listParams{}),
		Sort:       lp.Sort,
		Dir:        lp.Dir,
		Columns:    cols,
		Rows:       rows,
		Total:      res.Total,
		Page:       lp.Page,
		TotalPages: totalPages,
	}
	if lp.Page > 1 {
		prev := lp
		prev.Page--
		view.PrevURL = rs.listURL(prev)
	}
	if lp.Page < totalPages {
		next := lp
		next.Page++
		view.NextURL = rs.listURL(next)
	}
	return view
}

// notice maps the ?notice= value to a message. Only known keys are shown, so
// the parameter can never inject text into the page.
func (rs *resource[T]) notice(kind string) string {
	switch kind {
	case "created", "updated", "deleted":
		return i18n.T("notice." + kind)
	}
	return ""
}

func (rs *resource[T]) redirectWithNotice(w http.ResponseWriter, r *http.Request, kind string) {
	http.Redirect(w, r, rs.listURL(listParams{})+"?notice="+kind, http.StatusSeeOther)
}

// --- create and edit ---

func (rs *resource[T]) newForm(w http.ResponseWriter, r *http.Request, u User) {
	rs.renderForm(w, r, u, http.StatusOK, rs.newTitle(), rs.panel.url("/"+rs.slug), nil, "")
}

func (rs *resource[T]) create(w http.ResponseWriter, r *http.Request, u User) {
	if !rs.parseForm(w, r) {
		return
	}
	defer cleanUpload(r)
	var item T
	values, valid, err := rs.apply(r, &item)
	if err != nil {
		rs.panel.serverError(w, err)
		return
	}
	if !valid {
		rs.renderForm(w, r, u, http.StatusUnprocessableEntity, rs.newTitle(), rs.panel.url("/"+rs.slug), values, i18n.T("form.error_summary"))
		return
	}
	if err := rs.source.Create(r.Context(), &item); err != nil {
		rs.panel.serverError(w, err)
		return
	}
	rs.redirectWithNotice(w, r, "created")
}

func (rs *resource[T]) editForm(w http.ResponseWriter, r *http.Request, u User) {
	id := r.PathValue("id")
	item, err := rs.source.Find(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		rs.notFound(w, r, u)
		return
	}
	if err != nil {
		rs.panel.serverError(w, err)
		return
	}
	rv := reflect.ValueOf(item).Elem()
	values := make(map[string]form.Value, len(rs.fields))
	for i, f := range rs.fields {
		values[f.Info().Name] = form.Value{Raw: f.Format(rv.FieldByIndex(rs.fieldIdx[i]).Interface())}
	}
	rs.renderForm(w, r, u, http.StatusOK, rs.editTitle(), rs.itemURL(id), values, "")
}

func (rs *resource[T]) update(w http.ResponseWriter, r *http.Request, u User) {
	if !rs.parseForm(w, r) {
		return
	}
	defer cleanUpload(r)
	id := r.PathValue("id")
	item, err := rs.source.Find(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		rs.notFound(w, r, u)
		return
	}
	if err != nil {
		rs.panel.serverError(w, err)
		return
	}
	// Only the declared form fields are copied onto the record, so extra posted
	// values such as ID or CreatedAt are ignored.
	values, valid, err := rs.apply(r, item)
	if err != nil {
		rs.panel.serverError(w, err)
		return
	}
	if !valid {
		rs.renderForm(w, r, u, http.StatusUnprocessableEntity, rs.editTitle(), rs.itemURL(id), values, i18n.T("form.error_summary"))
		return
	}
	err = rs.source.Update(r.Context(), item)
	if errors.Is(err, ErrNotFound) {
		rs.notFound(w, r, u)
		return
	}
	if err != nil {
		rs.panel.serverError(w, err)
		return
	}
	rs.redirectWithNotice(w, r, "updated")
}

func (rs *resource[T]) delete(w http.ResponseWriter, r *http.Request, _ User) {
	if err := rs.source.Delete(r.Context(), r.PathValue("id")); err != nil {
		rs.panel.serverError(w, err)
		return
	}
	rs.redirectWithNotice(w, r, "deleted")
}

// apply copies the submitted values of the declared fields onto item. It
// returns each field's raw input with its validation message and whether every
// field was valid.
//
// A FileField (Image) is handled differently from the rest: its value comes
// from the uploaded file, not from POST text, and when the request carries no
// new file for it, the field's current value on item is kept rather than
// cleared, so editing a record without replacing its photo leaves it alone.
func (rs *resource[T]) apply(r *http.Request, item *T) (map[string]form.Value, bool, error) {
	values := make(map[string]form.Value, len(rs.fields))
	valid := true
	rv := reflect.ValueOf(item).Elem()
	for i, f := range rs.fields {
		name := f.Info().Name
		var raw string
		var parsed any
		var msg string

		if ff, ok := f.(form.FileField); ok {
			current := rv.FieldByIndex(rs.fieldIdx[i]).Interface()
			if fh := firstUploadedFile(r, name); fh != nil {
				parsed, msg = ff.ParseFile(r.Context(), fh, rs.fieldType[i], rs.panel.cfg.Storage)
				if msg == "" {
					raw = ff.Format(parsed)
				} else {
					raw = ff.Format(current) // rejected upload: keep showing the old image
				}
			} else {
				parsed, raw = current, ff.Format(current)
			}
		} else {
			raw = r.PostForm.Get(name)
			parsed, msg = f.Parse(raw, rs.fieldType[i])
		}

		values[name] = form.Value{Raw: raw, Error: msg}
		if msg != "" {
			valid = false
			continue
		}
		pv := reflect.ValueOf(parsed)
		if !pv.IsValid() || !pv.Type().AssignableTo(rs.fieldType[i]) {
			return nil, false, fmt.Errorf("tellus: field %q returned %T, want %s", name, parsed, rs.fieldType[i])
		}
		rv.FieldByIndex(rs.fieldIdx[i]).Set(pv)
	}
	return values, valid, nil
}

// firstUploadedFile returns the first file posted for name, or nil when the
// request has no multipart body or no file was chosen for that field.
func firstUploadedFile(r *http.Request, name string) *multipart.FileHeader {
	if r.MultipartForm == nil {
		return nil
	}
	fhs := r.MultipartForm.File[name]
	if len(fhs) == 0 {
		return nil
	}
	return fhs[0]
}

// cleanUpload removes any temporary files a multipart parse spilled to disk.
// Uploads under Config.MaxBodyBytes normally stay in memory, so this is
// usually a no-op; it exists for hosts that raise the limit.
func cleanUpload(r *http.Request) {
	if r.MultipartForm != nil {
		r.MultipartForm.RemoveAll()
	}
}

func (rs *resource[T]) renderForm(w http.ResponseWriter, r *http.Request, u User, status int, heading, action string, values map[string]form.Value, summary string) {
	comps := make([]templ.Component, len(rs.fields))
	for i, f := range rs.fields {
		v := values[f.Info().Name]
		if loader := rs.optionLoaders[i]; loader != nil {
			opts, err := loader(r.Context())
			if err != nil {
				rs.panel.serverError(w, err)
				return
			}
			comps[i] = f.(form.OptionsField).RenderOptions(v, opts)
			continue
		}
		comps[i] = f.Render(v)
	}
	rs.panel.render(w, r, status, ui.FormPage(ui.FormView{
		Shell:     rs.panel.shell(r, u, heading, rs.slug),
		Heading:   heading,
		Action:    action,
		CancelURL: rs.listURL(listParams{}),
		Error:     summary,
		Fields:    comps,
	}))
}

// maxMultipartMemory bounds how much of a multipart body ParseMultipartForm
// keeps in memory before spilling the rest to temporary files. It matches the
// default MaxBodyBytes, so uploads within the default limit never touch disk;
// a host that raises MaxBodyBytes may see files spill, cleaned up by
// cleanUpload after the request.
const maxMultipartMemory = defaultMaxBodyBytes

// parseForm parses the request body, files included. It answers the client
// itself and returns false when the body is unreadable or over the size
// limit. A request that is not multipart (no Image field was submitted) is
// parsed as an ordinary form, not an error.
func (rs *resource[T]) parseForm(w http.ResponseWriter, r *http.Request) bool {
	err := r.ParseMultipartForm(maxMultipartMemory)
	if err != nil && !errors.Is(err, http.ErrNotMultipart) {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, i18n.T("error.too_large"), http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, i18n.T("error.bad_request"), http.StatusBadRequest)
		}
		return false
	}
	return true
}

func (rs *resource[T]) notFound(w http.ResponseWriter, r *http.Request, u User) {
	rs.panel.renderMessage(w, r, u, http.StatusNotFound, i18n.T("common.not_found"), "")
}

func (rs *resource[T]) itemURL(id string) string {
	return rs.panel.url("/" + rs.slug + "/" + url.PathEscape(id))
}

func (rs *resource[T]) newTitle() string {
	return fmt.Sprintf(i18n.T("form.new_title"), rs.singular)
}

func (rs *resource[T]) editTitle() string {
	return fmt.Sprintf(i18n.T("form.edit_title"), rs.singular)
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
