package i18n

var en = map[string]string{
	// validation
	"validation.required":   "This field is required.",
	"validation.max_length": "Must be at most %d characters.",
	"validation.number":     "Enter a valid number.",

	// common
	"common.yes":       "Yes",
	"common.no":        "No",
	"common.not_found": "Not found.",

	// layout
	"ui.navigation":   "Main navigation",
	"ui.theme":        "Theme",
	"ui.toggle_theme": "Switch between light and dark theme",
	"ui.sign_out":     "Sign out",
	"ui.no_resources": "No resources are registered yet.",

	// login
	"login.title":     "Sign in",
	"login.lede":      "Sign in to continue.",
	"login.email":     "Email",
	"login.password":  "Password",
	"login.submit":    "Sign in",
	"login.invalid":   "Invalid email or password.",
	"login.too_many":  "Too many attempts. Try again in a few minutes.",
	"login.forbidden": "This account cannot access the panel.",

	// list
	"list.new":            "New",
	"list.search":         "Search",
	"list.empty":          "Nothing here yet.",
	"list.actions":        "Actions",
	"list.edit":           "Edit",
	"list.delete":         "Delete",
	"list.confirm_delete": "Delete this record? This cannot be undone.",
	"list.summary":        "%d records, page %d of %d",
	"list.previous":       "Previous",
	"list.next":           "Next",

	// form
	"form.new_title":     "New %s",
	"form.edit_title":    "Edit %s",
	"form.save":          "Save",
	"form.cancel":        "Cancel",
	"form.error_summary": "Some fields need attention.",

	// notices shown after a redirect
	"notice.created": "Record created.",
	"notice.updated": "Record saved.",
	"notice.deleted": "Record deleted.",
}
