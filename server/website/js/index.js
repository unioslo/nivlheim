var userinfo;

$(document).ready(function(){
	Handlebars.registerHelper('formatDateTime', function(s){
		if (!s) return "";
		let t = moment(s);
		let format = 'D MMM Y HH:mm';
		if (Math.abs(t.diff(new Date(), 'days')) > 7) {
			format = 'D MMM Y'; // omit the time of day
		}
		let str = '<span class="nobreak">'+t.fromNow()+'</span> '
				+ '<span class="nobreak">(' + t.format(format) + ')</span>';
		//str = Handlebars.Utils.escapeExpression(str);
		if (t.isAfter(moment().subtract(1,'days'))) {
			return new Handlebars.SafeString(str);
		} else {
			return new Handlebars.SafeString(
				'<span class="underline-warning">'+str+'</span>');
		}
	});
	Handlebars.registerHelper('formatInterval', function(seconds){
		if (!seconds || seconds <= 0) return "0";
		let epoch = Math.floor((new Date).getTime()/1000);
		let m = moment.unix(epoch-seconds);
		let str = m.fromNow(true);
		return Handlebars.Utils.escapeExpression(str);
	});
	Handlebars.registerHelper('formatNumber', function(number){
		if (typeof number == "number") {
			if (Math.abs(number)>=999) return Math.round(number);
			return number.toPrecision(3);
		}
		return "-";
	});
	Handlebars.registerHelper('urlescape', function(s){
		if (!s) return "";
		return encodeURIComponent(s);
	});
	Handlebars.registerHelper('ifmatch', function(a,b,options){
		if (a.match(b)) return options.fn(this);
		return options.inverse(this);
	});
	Handlebars.registerHelper('ifcmp', function(a,operator,b,options){
		switch (operator) {
			case '=': if (a == b) return options.fn(this); break;
			case '!=': if (a != b) return options.fn(this); break;
			case '<': if (a < b) return options.fn(this); break;
			case '>': if (a > b) return options.fn(this); break;
		}
		return options.inverse(this);
	});
	Handlebars.registerHelper('pagination', function(page, maxPage, block){
		let accum = '';
		if (page-5>1) {
			accum += block.fn(1);
		}
		if (page-5>2) {
			accum += block.fn('...');
		}
		let from = Math.max(page-5, 1);
		let to = Math.min(Math.max(page+5, 10), maxPage);
		for(let i = from; i <= to; ++i)
			accum += block.fn(i);
		if (to < maxPage-1) {
			accum += block.fn('...');
		}
		if (to < maxPage) {
			accum += block.fn(maxPage);
		}
		return accum;
	});
	Handlebars.registerHelper('previous', function(n,block){
		if (n > 1) { return block.fn(n-1); }
		return "";
	});
	Handlebars.registerHelper('next', function(n,m,block){
		if (n<m) { return block.fn(n+1); }
		return "";
	});

	var routes = {
		'/allhosts': allHosts,
		'/browsehost/:certfp': browseHostByCert,
		'/deletehost/:certfp': deleteHostByCert,
		'/browsefile/:fileId': browseFileById,
		'/browsefile/:certfp/:filename': browseFileByName,
		'/search/:page/:query': searchPage,
		'/search': searchPage,
		'/settings/ipranges': iprangesPage,
		'/settings': settingsPage,
		'/keys': keysPage,
		'/keys/:keyId': keyEditPage,
		'/': showFrontPage
	};

	var router = new tarantino.Router(routes);

	router.configure({
		html5history: false,
		notfound: function(){
			$("div#pageContent").html('<section class="section">'
				+'<i class="fas fa-question fa-2x"></i>'
				+'<i class="fas fa-exclamation fa-2x"></i> '
				+'Route not found...</section>');
		}
	});

	router.param('fileId', /(\\d+)/);
	router.param('certfp', /([0-9A-F]{32,40})/);
	//router.param('hostname', /([\\w\\.]+\\w+)/);
	router.param('filename', /([A-Za-z0-9_\\.~\\-]+)/);
	router.param('query', /(.+)/);
	router.param('page', /(\d+)/);
	router.param('keyId', /(\\d+)/);

	// handle the "burger" menu icon that appears on narrow screens
	$("div.navbar-burger").click(function(){
		$(this).toggleClass('is-active');
		$("div.navbar-menu").toggleClass('is-active');
	});
	$("a.navbar-item").click(function(){
		$("div.navbar-burger").removeClass('is-active');
		$("div.navbar-menu").removeClass('is-active');
	});

	// attach a handler that will load more elements when scrolling
	$(window).scroll(scrollHandler);
	window.setInterval(scrollHandler,500);

	// show the name of the logged in user, or redirect to login
	$.getJSON(getAPIURLprefix()+"/api/v2/userinfo", function(data){
		userinfo = data;
		if (data == null) {
			// Not logged in. Redirect to login...
			location.href = getAPIURLprefix()+"/api/oauth2/start"
				+"?redirect="+encodeURIComponent(location.href);
			return
		} else if (data.name) {
			// Logged in
			$("a#loginLink").remove();
			$("div#loggedInUser").removeClass("is-not-displayed")
			$("div#loggedInUser span#fullname").text(data.name);
			$("a#logoutLink").prop("href", getAPIURLprefix()+"/api/oauth2/logout");
		} else if (data.authDisabled) {
			// Authentication is not enabled
		}
		// At this point, authentication is definitely taken care of.
		router.init('/'); // Initialize the router (Tarantino) and go to the front page
		// retrieve the version number from a static file and display it
		$.get('/version.txt', function(data){
			$("span#navbarVersion").text('Version ' + data);
		});
	});

	// create a shake effect
	jQuery.fn.shake = function() {
		this.each(function() {
			$(this).css({
				"position": "relative"
			}).animate({left:20},30).animate({left:-20},30).animate({left:0},30);
		});
		return this;
	}
});

function showFrontPage() {
	document.title = "Home - Nivlheim";
	renderTemplate("frontpage", {}, "div#pageContent")
	.done(function(){
		$("button#searchButton").click(newSearch);
		$("input#search").keyup(function(e){
			if(e.keyCode===13){newSearch();}
		});
		autoReloadStatus();
	});
}

function browseHostByCert(certfp) {
	// First, get a list of custom fields (if any)
	let customfields = [];
	$.get(getAPIURLprefix()+"/api/v2/settings/customfields?fields=name",
	function(data){
		for (let i=0; i<data.length; i++) {
			customfields[i] = data[i].name;
		}
	})
	.done(function(){
		APIcall(
			//"mockapi/browsehost.json",
			"/api/v2/host/"+encodeURIComponent(certfp)+
			"?fields=ipAddress,hostname,overrideHostname,lastseen,os,osEdition,osFamily,"+
			"kernel,manufacturer,product,serialNo,clientVersion,certfp,files,"+
				customfields.join(","), // also ask for the custom fields
			"browsehost", "div#pageContent",
			function(data){
				document.title = data['hostname'] + " - Nivlheim";
				// put the custom fields in a map so they can be iterated through
				let m = {};
				for (let i=0; i<customfields.length; i++) {
					let name = customfields[i];
					m[name] = data[name];
				}
				data["customfields"] = m;
				return data;
			})
		.done(function(){
			window.scrollTo(0,0);
			attachHandlersToForms();
		});
	});
}

function deleteHostByCert(certfp) {
	APIcall(
		"/api/v2/host/"+encodeURIComponent(certfp)+
		"?fields=ipAddress,hostname,lastseen,os,osEdition,"+
		"manufacturer,product,certfp",
		"deletehost", "div#pageContent",
	function(data){
		var options = [
			{"value": "0", "label": "No"},
			{"value": "0", "label": "Maybe"},
			{"value": "0", "label": "90%"},
			{"value": "0", "label": "I don't know"},
			{"value": "1", "label": "Yes"},
		];
		shuffleArray(options);
		data["options"] = options;
		return data;
	})
	.done(function(){
		$("a#deleteButton").click(function(){
			if ($("input[name='sure']:checked").val()==1) {
				restDeleteHost(certfp);
			} else {
				$("a#deleteButton").shake();
			}
		});
		$("a#cancelButton").click(function(){
			history.back();
		});
	});
}

function restDeleteHost(certfp) {
	// Put a spinner on the button
	$("a#deleteButton").addClass("is-loading");
	// Perform the ajax call
	let url = getAPIURLprefix()+"/api/v2/host/"+certfp;
	$.ajax({
		"url": url,
		"method": "DELETE"
	})
	.fail(function(jqxhr){
		// Error. Display error messages, if any
		let text = jqxhr.statusCode().responseText;
		alert(text);
		// Remove the spinner
		$("a#deleteButton").removeClass("is-loading");
	})
	.done(function(){
		// Success. Change the button
		$("a#deleteButton").replaceWith('<a class="button">The machine has been deleted.</a>');
		// Fade out the details
		$("div#machinedetails").fadeOut(1500);
		$("a#cancelButton").fadeOut(1500);
	});
}

function browseFileById(fileId) {
	APIcall(
		//"mockapi/browsefile.json",
		"/api/v2/file?fields=fileId,lastModified,hostname,filename,"+
		"content,certfp,versions,isNewestVersion,isDeleted"+
		"&fileId="+encodeURIComponent(fileId),
		"browsefile", "div#pageContent")
	.done(showDiff)
	.done(function(){
		$("select#selectVersion").val(fileId);
		$("select#selectVersion").change(function(){
			location.href = "#/browsefile/"+$(this).val();
		});
		window.scrollTo(0,0);
	});
}

function browseFileByName(certfp, filename) {
	filename = decodeURIComponent(filename);
	APIcall(
		//"mockapi/browsefile.json",
		"/api/v2/file?fields=fileId,lastModified,"+
		"hostname,filename,content,certfp,versions,"+
		"isNewestVersion,isDeleted"+
		"&filename="+encodeURIComponent(filename)+
		"&certfp="+certfp,
		"browsefile", "div#pageContent")
	.done(showDiff)
	.done(function(data){
		$("select#selectVersion:first-child").prop("selected","selected");
		$("select#selectVersion").change(function(){
			location.href = "#/browsefile/"+$(this).val();
		});
		window.scrollTo(0,0);
		document.title = data['filename'] + " - Nivlheim";
	});
}

function newSearch() {
	let q = $("input#search").val();
	if (q == "") return;
	let new_href = "#/search/1/" + encodeURIComponent(q);
	if (location.href.endsWith(new_href)) {
		// The user clicked "search" again, with the same query as before.
		// Let's reload the search, then.
		searchPage(1, encodeURIComponent(q));
	} else {
		location.href = new_href;
	}
}

function searchPage(page, q) {
	if (!q) q = "";
	else q = decodeURIComponent(q);
	// if we're not already on the search page, render the template
	renderTemplate("searchpage", {"query": q}, "div#pageContent")
	.done(function(){
		document.title = 'Search - Nivlheim';
		// add handlers to the input field and button
		$("button#searchButton").click(newSearch);
		$("input#search").keyup(function(e){
			if(e.keyCode===13){newSearch();}
		}).focus(); // focus the input field
		// no query? then exit
		if (q == "") return;
		// show the spinner
		$("div#searchSpinner").fadeIn();
		// search host names
		APIcall("/api/v2/hostlist?fields=hostname,certfp&hostname="+
			encodeURIComponent("*"+q.replace(' ','*')+"*"), "searchresulthostnames",
			"div#searchResultHostnames");
		// search IP addresses
		APIcall("/api/v2/hostlist?fields=hostname,certfp&ipAddress="+
			encodeURIComponent("*"+q.replace(' ','*')+"*"), "searchresulthostnames",
			"div#searchResultHostnames2");
		// search files
		APIcall(
			//"mockapi/searchpage.json",
			"/api/v2/searchpage?q="+encodeURIComponent(q)+
			"&page="+page+"&hitsPerPage=8",
			"searchresultfiles", "div#searchResult")
		.always(function(){
			// hide the spinner
			$("div#searchSpinner").hide();
			// move the text cursor to the end of the search input box
			let elem = $("input#search")[0];
			elem.selectionStart = elem.selectionEnd = elem.value.length;
		});
		document.title = q + ' - Nivlheim search';
	});
}

function allHosts() {
	// are we already on the browse page? then don't reload it
	if ($("aside.menu").length>0) return;
	// retrieve lists of OSes, Manufacturers, etc.
	let pfx = getAPIURLprefix();
	let promises = [];
	promises.push($.get(pfx+"/api/v2/hostlist?group=os"));
	promises.push($.get(pfx+"/api/v2/hostlist?group=osEdition"));
	promises.push($.get(pfx+"/api/v2/hostlist?group=manufacturer"));
	promises.push($.get(pfx+"/api/v2/hostlist?group=product"));
	// wait for all the promises to complete
	$.when.apply($, promises).then(function(){
		// remove entries that are the string "null"
		for (let i=0; i<arguments.length; i++)
			delete arguments[i][0]["null"];
		// compose an object to send to the handlebars template
		var data = {
			"os": arguments[0][0],
			"osEdition": arguments[1][0],
			"manufacturer": arguments[2][0],
			"product": arguments[3][0],
		};
		renderTemplate("allhosts", data, "div#pageContent")
		.done(function(){
			// Look at url parameters (if any) and select appropriate items
			let p = getUrlParams();
			if (p["q"]) {
				let a = p["q"].split('|');
				for (let i=0; i<a.length; i++) {
					$("aside.menu li a:contains('"+a[i]+"')").addClass('is-active');
				}
			}
			// Load matching hosts
			reloadMatchingHosts();
			// Add event handlers
			$("aside.menu li a").click(function(){
				// toggle this item on/off
				$(this).toggleClass('is-active');
				reloadMatchingHosts();
			});
		});
		document.title = 'Browse hosts - Nivlheim';
	})
	.fail(function(){
		showError("Something went wrong when querying the server.",
			"div#pageContent", "fa-sad-cry");
	});
}

function reloadMatchingHosts() {
	// find the selected elements from the menu
	let oses = [];
	$("aside.menu li a.os.is-active span:first-of-type").each(function(i,e){
		oses.push(e.innerText.replace(",","%2C"));
	});
	let editions = [];
	$("aside.menu li a.osEdition.is-active span:first-of-type").each(function(i,e){
		editions.push(e.innerText.replace(",","%2C"));
	});
	let manufacturers = [];
	$("aside.menu li a.manufacturer.is-active span:first-of-type").each(function(i,e){
		manufacturers.push(e.innerText.replace(",","%2C"));
	});
	let products = [];
	$("aside.menu li a.product.is-active span:first-of-type").each(function(i,e){
		products.push(e.innerText.replace(",","%2C"));
	});
	// set the query string in the url
	let q = oses.concat(editions).concat(manufacturers).concat(products).join('|');
	if (q) q = "?q="+q;
	location.assign("/#/allhosts"+q);
	// prepare the API call that loads the list of hosts that match
	q = "/api/v2/hostlist?fields=hostname,ipAddress,certfp";
	if (oses.length>0) q += "&os="+oses.join(',');
	if (editions.length>0) q += "&osEdition="+editions.join(',');
	if (manufacturers.length>0) q += "&manufacturer="+manufacturers.join(',');
	if (products.length>0) q += "&product="+products.join(',');
	$("div#hostlist").data("query",q).data("offset",0).html("");
	loadMoreHosts();
}

function loadMoreHosts() {
	let q = $("div#hostlist").data("query");
	let offset = $("div#hostlist").data("offset") || 0;
	//console.log("Loading from "+offset);
	let limit = 30;
	APIcall(q + "&limit="+limit+"&offset="+offset, "hostlist",
		"div#morehosts")
	.done(function(){
		$("div#hostlist").data("offset", offset + limit);
		$("div#morehosts").children().appendTo("div#hostlist");
		$("div.loadmore").scroll(loadMoreHosts);
	});
}

function settingsPage() {
	document.title = "Settings - Nivlheim";
	renderTemplate("settingspage", {}, "div#pageContent")
	.done(function(){
		let p1 = APIcall(
			//"mockapi/awaiting_approval.json",
			"/api/v2/manualApproval"+
			"?fields=hostname,reversedns,ipaddress,approvalId&approved=null",
			"awaiting_approval", $('#placeholder_approval'))
			.done(function(){
				attachHandlersToDenyAndAcceptButtons();
			});
		let p2 = APIcall(
			"/api/v2/settings/customfields?fields=name,filename,regexp",
			"customfields", "#placeholder_customfields");
		Promise.all([p1,p2]).then(function(){
			$("#resetWaitTimeButton").click(function(){restPut('/api/v2','resetWaitingTimeForFailedTasks','')});
			attachHandlersToForms();
		});
	});
}

function iprangesPage() {
	document.title = "IP ranges - Nivlheim";
	APIcall(//"mockapi/ipranges.json",
		"/api/v2/settings/ipranges?fields=ipRangeId,ipRange,comment,useDns",
		"ipranges", "div#pageContent")
	.done(function(){
		attachHandlersToForms();
	});
}

function keysPage() {
	document.title = "API keys - Nivlheim";
	APIcall("/api/v2/keys?fields=keyID,key,comment,filter,readonly,expires,ipRanges",
		"keyspage", "div#pageContent")
	.done(function(){
		attachHandlersToForms();
		let j = window.location.href.indexOf("/", 10);
		$("span#apiPrefix").text(window.location.href.substr(0,j)+"/api/v2/");
	});
}

function keyEditPage(keyid) {
	document.title = "API keys - Nivlheim";
	APIcall("/api/v2/keys/"+keyid+"?fields=keyID,key,comment,filter,readonly,expires,ipRanges", 
		"keyeditpage", "div#pageContent", function(obj){
			// Only show the expiry date, not the whole timestamp
			if (obj["expires"] && obj["expires"].length>10)
				obj["expires"] = obj["expires"].substr(0,10);
			return obj;
		})
	.done(function(){
		attachHandlersToForms();
	});
}