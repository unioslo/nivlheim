//  http://berzniz.com/post/24743062344/handling-handlebarsjs-like-a-pro
function renderTemplate(name, templateValues, callback) {
	if (Handlebars.templates === undefined || Handlebars.templates[name] === undefined) {
		// must load and compile the template first
		$.ajax({
			url : 'templates/' + name + '.handlebars',
			success : function(data) {
				// compile and keep
				if (Handlebars.templates === undefined) {
					Handlebars.templates = {};
				}
				console.log("Compiling " + name + ".handlebars");
				Handlebars.templates[name] = Handlebars.compile(data, {"strict":true});
				// now, run the template
				var output = Handlebars.templates[name](templateValues);
				callback(output);
			},
			dataType : 'text'
		}).fail(function(jqxhr, textStatus, error){
			throw error;
		});
	} else {
		callback(Handlebars.templates[name](templateValues));
	}
}

function showError(error, domElement, faIconName) {
	$(domElement).html('<nav class="level"><div class="level-left">'
		+'<div class="level-item"><span class="icon">'
		+'<i class="fas fa-lg ' + faIconName + '"></i></span>'
		+'</div><div class="level-item">'+error+'</div></div></nav>');
}

function APIcall(url, templateName, domElement) {
	var deferredObj = $.Deferred();
	$.getJSON(url, function(data){
		try {
			renderTemplate(templateName, data, function(output){
				$(domElement).html(output);
				deferredObj.resolve();
			});
		}
		catch(error) {
			showError(error, domElement, "fa-exclamation-triangle");
			deferredObj.resolve();
		}
	})
	.fail(function(jqxhr, textStatus, error){
		if (jqxhr.status == 404)
			showError(error, domElement, "fa-unlink");
		else
			showError(error, domElement, "fa-exclamation-circle");
		deferredObj.resolve();
	});
	return deferredObj;
}

// Reads the page's URL parameters and returns them as an associative array
function getUrlParams() {
	var vars = [];
	var start = window.location.href.indexOf('?') + 1;
	if (start == 0) { return vars; }
	var end = window.location.href.indexOf('#');
	if (end < 0) end = window.location.href.length;
	var pairs = window.location.href.slice(start,end).split('&');
	for (var i = 0; i < pairs.length; i++) {
		pair = pairs[i].split('=');
		vars[decodeURIComponent(pair[0])] = decodeURIComponent(pair[1].replace(/\+/g,' '));
	}
	return vars;
}
