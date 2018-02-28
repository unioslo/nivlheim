function browseHost(certfp, pushState = true) {
	if (pushState) {
		history.pushState({"certfp":certfp}, null, "/browse.html?c="+
			encodeURIComponent(certfp));
	}
	APIcall(
		//"mockapi/browsehost.json",
		"/api/v0/host?certfp="+encodeURIComponent(certfp)+
		"&fields=ipAddress,hostname,lastseen,os,osEdition,"+
		"kernel,vendor,model,serialNo,clientVersion,certfp,files",
		"browsehost", "#placeholder_browse");
}

function browseFile(fileId, pushState = true) {
	if (pushState) {
		history.pushState({"fileId":fileId}, null, "/browse.html?f="+
			encodeURIComponent(fileId));
	}
	APIcall(
		//"mockapi/browsefile.json",
		"/api/v0/file?fields=fileId,lastModified,hostname,filename,"+
		"content,certfp,versions&fileId="+encodeURIComponent(fileId),
		"browsefile", "#placeholder_browse")
	.done(showDiff)
	.done(function(){
		$("select#selectVersion").val(fileId);
		$("select#selectVersion").change(function(){
			browseFile($(this).val());
		})
	});
}

function browseFile2(hostname, filename, pushState = true) {
	if (pushState) {
		history.pushState({
				"filename":filename,
				"hostname":hostname
			}, null,
			"/browse.html?fn="+encodeURIComponent(filename)+
			"&h="+encodeURIComponent(hostname));
	}
	APIcall(
		//"mockapi/browsefile.json",
		"/api/v0/file?fields=fileId,lastModified,"+
		"hostname,filename,content,certfp,versions"+
		"&filename="+encodeURIComponent(filename)+
		"&hostname="+encodeURIComponent(hostname),
		"browsefile", "#placeholder_browse")
	.done(showDiff)
	.done(function(){
		$("select#selectVersion:first-child").prop("selected","selected");
		$("select#selectVersion").change(function(){
			browseFile($(this).val());
		})
	});
}

function showDiff(data) {
	// We got the first file, let's get the second one and compare
	// Find the ID of the previous version
	let otherFileId = 0;
	for (var i=0; i<data.versions.length-1; i++) {
		if (data.versions[i].fileId == data.fileId) {
			otherFileId = data.versions[i+1].fileId;
			break;
		}
	}
	if (otherFileId == 0) {
		// Don't have any previous version to compare with.
		return;
	}
	let urlprefix = '';
	if (location.origin.match('http://(127\\.0\\.0\\.1|localhost)')) {
		// Developer mode. Assumes the API is running locally on port 4040.
		urlprefix = "http://localhost:4040";
	}
	// Retrieve the contents of the previous version
	$.getJSON(urlprefix+"/api/v0/file?fileId="+otherFileId+"&fields=content",
		function(data2){
			$("div.filecontent").html(diffString(
				htmlEntities(data2.content),
				htmlEntities(data.content)));
		})
		.fail(function(jqxhr, textStatus){
			console.log(jqxhr.status + ' ' + jqxhr.statusCode().responseText);
		});
}

function htmlEntities(str) {
	return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;')
		.replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function navigateByUrlParams() {
	var p = getUrlParams();
	if (p['c']) {
		browseHost(p['c'], false);
	} else if (p['f']) {
		browseFile(p['f'], false);
	} else if (p['h'] && p['fn']) {
		browseFile2(p['h'], p['fn'], false);
	}
}

function readyFunc() {
	Handlebars.registerHelper('formatDateTime', function(s){
		if (!s) return "";
		var t = moment(s);
		return t.fromNow() + ' (' + t.format('D MMM Y HH:mm') + ')';
	});
	window.addEventListener('popstate', popstate);
	navigateByUrlParams();
}

function popstate(e) {
	if (e.state) {
		if (e.state.certfp) {
			browseHost(e.state.certfp, false);
		} else if (e.state.fileId) {
			browseFile(e.state.fileId, false);
		} else if (e.state.hostname) {
			browseFile2(e.state.hostname, e.state.filename, false);
		}
	} else {
		navigateByUrlParams();
	}
}

$(document).ready(readyFunc);
