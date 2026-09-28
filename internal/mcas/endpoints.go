package mcas

const (
	baseURL         = "https://www.mychildatschool.com"
	loginPath       = "/MCAS/MCSParentLogin"
	dashboardMarker = "MCSDashboard"
	// Generic server-side proxy onto the api/v1 REST backend. Every data call
	// goes through this: {"url": "api/v1/...", ...} -> {"d": "<json string or html>"}.
	proxyPath = "/MCAS/WebServices/MCSAPIRequestProxy.asmx/CreateGetRequest"
	// The timetable and academic calendar pages are server-rendered - no API
	// call exists for them, so the week grid is parsed out of the page itself.
	timetablePage = "/MCAS/MCSTimetable.aspx"
)

// api/v1 routes. The full route map is a public file served by the portal at
// /Scripts/Blankon/Custom/apiEndPoints.js - consult it before inventing a path.
const (
	epUserDetails     = "api/v1/mcas/user/details"
	epStudentYears    = "api/v1/mcas/homework/studentYears/%d"
	epAttendance      = "api/v1/attendance/mcas/tableRawData/%d/%d/%d/%d/-1"
	epBehaviour       = "api/v1/eventRecords/mcas/eventstable/%d/%d/%d/%d/%d/-1"
	epBehaviourDetail = "api/v1/eventRecords/mcas/eventdetails/%d/%d"
	epDetentions      = "api/v1/detentions/mcas/%d"
	epDinner          = "api/v1/mcas/dashboard/GetDinnerBalanceWidgetData/%d"
	epConfigurations  = "api/v1/mcas/configurations"
	epReports         = "api/v1/studentDetails/reports/%d"
	epClubs           = "api/v1/mcas/clubsandtrips/StudentClubsAndTrips/%d"
)

// moduleFlags maps a module name to the configuration key that reports
// whether the school has licensed it. Schools license MCAS modules
// individually; a sensor/column that can only ever read zero because the
// school never enabled a module is worse than not showing it at all.
var moduleFlags = map[string]string{
	"attendance": "MCASAttendanceModuleEnabled",
	"behaviour":  "MCASBehaviourModuleEnabled",
	"detentions": "MCASEnableDetentions",
	"timetable":  "MCASTimetableModuleEnabled",
	"reports":    "MCASReportsModuleEnabled",
	"dinner":     "MCASDinnerMoneyModule_EnableDinnerMoneyModule",
	"clubs":      "MCASClubsModuleEnabled",
	"trips":      "MCASTripsModuleEnabled",
}

// dayStatus maps DayStatusCode values from the behaviour year payload's
// calendar table.
var dayStatus = map[string]DayType{
	"-": SchoolDay,
	"*": Weekend,
	"#": Holiday,
	"$": StaffDay,
}

// presentMarkSigns are the marks that count as the pupil being in school.
var presentMarkSigns = map[string]bool{"P": true, "L": true}

// The proxy reports a bad path as HTTP 200 with this string in "d", so it
// has to be sniffed rather than caught as an error.
const notFoundMarker = "Error 404"

// userAgent identifies this CLI to the portal, distinct from the reference
// Home Assistant integration's "HomeAssistant-MCAS".
const userAgent = "my-child-at-school-cli/0.1"
