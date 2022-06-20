(use-modules (ice-9 textual-ports)
             (git bindings)
             (git repository)
             (git reference)
             (guix packages)
             (guix gexp)
             ((guix git-download) #:select (git-predicate))
             (gnu packages certs)
             (uio packages nivlheim))

(define %checkout (dirname (current-filename)))

(define version
  (call-with-input-file (string-append %checkout "/VERSION")
    (lambda (port)
      (string-trim-right (get-string-all port)))))

(define ref
  (or (getenv "GITHUB_REF_NAME")
      (if (file-exists? (string-append %checkout "/.git"))
          (begin
            (libgit2-init!)
            (let* ((repo (repository-open %checkout))
                   (head (repository-head repo))
                   (branch (reference-shorthand head)))
              (repository-close! repo)
              (libgit2-shutdown!)
              branch))
          "unknown")))

(define (ref-is-tag? ref)
  (if (getenv "GITHUB_ACTIONS")
      (string=? (getenv "GITHUB_REF_TYPE") "tag")
      ;; XXX: guile-git lacks bindings for git_reference_type,
      ;; so we instead rely on the fact that REF becomes "HEAD"
      ;; when not on a branch.
      (string=? ref "HEAD")))

(define %nivlheim
  (package
    (inherit nivlheim)
    ;; Custom variant of Nivlheim that uses the local checkout as source,
    ;; and with "version" set to the contents of the VERSION file and
    ;; current branch.
    (version (if (or (string=? ref "master") (ref-is-tag? ref))
                 version
                 (string-append version "-" ref)))
    (source (local-file %checkout
                        #:recursive? #t
                        #:select? (git-predicate %checkout)))
    ;; Propagate nss-certs to ensure /etc/ssl/certs is available.
    (propagated-inputs
     (append (list nss-certs)
             (package-propagated-inputs nivlheim)))))

(packages->manifest
 (list %nivlheim))
