#!/usr/bin/env bash
# SPDX-License-Identifier: MIT

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
go_licenses_pkg="github.com/google/go-licenses/v2@v2.0.1"
output_root="${repo_root}/third_party_licenses"
exceptions_file="${repo_root}/scripts/license-exceptions.tsv"

runtime_output_dir="${output_root}/runtime"
runtime_report="${output_root}/runtime-report.csv"

mkdir -p "${output_root}"
rm -rf "${runtime_output_dir}"

# Modules go-licenses cannot classify; see scripts/license-exceptions.tsv.
exception_modules=()
exception_spdx=()
exception_files=()
while IFS=$'\t' read -r module spdx license_file _reason; do
	[[ -z "${module}" || "${module}" == \#* ]] && continue
	exception_modules+=("${module}")
	exception_spdx+=("${spdx}")
	exception_files+=("${license_file}")
done <"${exceptions_file}"

# Replace the "Unknown,Unknown" rows go-licenses emits for an excepted module
# with a single canonical row carrying the manually verified SPDX identifier.
rewrite_report_rows() {
	local report_path="$1" module="$2" spdx="$3" license_file="$4" version="$5"
	local tmp
	tmp="$(mktemp)"
	grep -v "^${module}\(/\|,\)" "${report_path}" >"${tmp}" || true
	printf '%s,https://%s/blob/%s/%s,%s\n' \
		"${module}" "${module}" "${version}" "${license_file}" "${spdx}" >>"${tmp}"
	LC_ALL=C sort "${tmp}" -o "${report_path}"
	rm -f "${tmp}"
}

# go-licenses skipped these modules, so copy their license files in ourselves.
save_exception_license() {
	local module="$1" license_file="$2" module_dir="$3" save_path="$4"
	local dest="${save_path}/${module}"
	mkdir -p "${dest}"
	cp "${module_dir}/${license_file}" "${dest}/${license_file}"
	chmod u+w "${dest}/${license_file}"
}

run_generation() {
	local module_dir="$1"
	local package_arg="$2"
	local ignore_prefix="$3"
	local report_path="$4"
	local save_path="$5"

	local ignore_args=(--ignore "${ignore_prefix}")
	local module
	for module in "${exception_modules[@]}"; do
		ignore_args+=(--ignore "${module}")
	done

	(
		cd "${module_dir}"
		go run "${go_licenses_pkg}" report "${package_arg}" --ignore "${ignore_prefix}" >"${report_path}"
		go run "${go_licenses_pkg}" save "${package_arg}" "${ignore_args[@]}" --save_path "${save_path}"

		local i dep_dir dep_version
		for i in "${!exception_modules[@]}"; do
			module="${exception_modules[${i}]}"
			dep_dir="$(go list -m -f '{{.Dir}}' "${module}")"
			dep_version="$(go list -m -f '{{.Version}}' "${module}")"
			rewrite_report_rows "${report_path}" "${module}" \
				"${exception_spdx[${i}]}" "${exception_files[${i}]}" "${dep_version}"
			save_exception_license "${module}" "${exception_files[${i}]}" \
				"${dep_dir}" "${save_path}"
		done
	)
}

run_generation "${repo_root}" "./cmd/sting" "github.com/skaphos/sting" "${runtime_report}" "${runtime_output_dir}"

printf 'Updated third-party notices in %s\n' "${output_root}"
