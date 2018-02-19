function browseHost(certfp) {
	APIcall(
		//"mockapi/browsehost.json",
		"http://127.0.0.1:4040/api/v0/host?certfp="+certfp+
		"&fields=ipAddress,hostname,lastseen,os,osEdition,"+
		"kernel,vendor,model,serialNo,clientVersion,certfp,files",
		"browsehost", "#placeholder_browse");
}

function browseFile(fileId) {
	APIcall(
		//"mockapi/browsefile.json",
		"http://127.0.0.1:4040/api/v0/file?fields=lastModified,hostname,filename,"+
		"content,certfp,versions&fileId="+fileId,
		"browsefile", "#placeholder_browse")
	.done(function(){
		$("select#selectVersion").val(fileId);
		$("select#selectVersion").change(function(){
			browseFile($(this).val());
		})
	});
}

function browseFile2(hostname, filename) {
	APIcall(
		//"mockapi/browsefile.json",
		"http://127.0.0.1:4040/api/v0/file?fields=fileId,lastModified,"+
		"hostname,filename,content,certfp,versions"+
		"&filename="+filename+"&hostname="+hostname,
		"browsefile", "#placeholder_browse")
	.done(function(){
		$("select#selectVersion:first-child").prop("selected","selected");
		$("select#selectVersion").change(function(){
			browseFile($(this).val());
		})
	});
}

function readyFunc() {
	Handlebars.registerHelper('formatDateTime', function(s){
		var t = moment(s);
		return t.fromNow() + ' (' + t.format('D MMM Y HH:mm') + ')';
	});
	var p = getUrlParams();
	if (p['c']) {
		browseHost(p['c']);
	} else if (p['f']) {
		browseFile(p['f']);
	} else if (p['h'] && p['fn']) {
		browseFile2(p['h'], p['fn']);
	}
}

$(document).ready(readyFunc);

/* this code doesn't work yet -- needs debugging
window.history.pushState(null, null, "/browse.html?f="+fileId);
window.onpopstate = function(event) {
	console.log("popstate");
	readyFunc();
}
*/
