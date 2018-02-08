$(document).ready(function(){
	var p = getUrlParams();
	if (p['c']) {
		APIcall("mockapi/browsehost.json", "browsehost",
			"#placeholder_browse");
	} else if (p['fid']) {
		APIcall("mockapi/browsefile.json", "browsefile",
			"#placeholder_browse");
	}
});
