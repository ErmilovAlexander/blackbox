#!/bin/sh

set -eu

NEXUS_BASE_URL=${NEXUS_BASE_URL:-https://mirror.ip-10-28-32-189.shturval.link}
HELM_REPOSITORY_URL=${HELM_REPOSITORY_URL:-${NEXUS_BASE_URL}/repository/shturval_helm/}
IMAGE_NAME=${IMAGE_NAME:-kube-blackbox}
IMAGE_TAG=${IMAGE_TAG:-4faa2b2}
IMAGE_DIGEST=${IMAGE_DIGEST:-sha256:5b7a52c94e04dbd65425db71da428f6a1175a10cf3b4c7596e204211e1c6d557}
CHART_VERSION=${CHART_VERSION:-0.1.3}
CHART_DIGEST=${CHART_DIGEST:-21a6cd0c24a81cd536908d94722afd8a725f1965d4e8dd022e90cbba1fff4253}

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(CDPATH= cd -- "${script_dir}/.." && pwd)
work_dir=$(mktemp -d)
trap 'rm -rf "${work_dir}"' EXIT HUP INT TERM

manifest_url=${NEXUS_BASE_URL}/v2/${IMAGE_NAME}/manifests/${IMAGE_TAG}
curl --fail --silent --show-error --insecure \
  --header 'Accept: application/vnd.oci.image.manifest.v1+json' \
  --dump-header "${work_dir}/manifest.headers" \
  --output "${work_dir}/manifest.json" \
  "${manifest_url}"

actual_image_digest=$(awk '
  tolower($1) == "docker-content-digest:" {
    gsub("\\r", "", $2)
    print $2
  }
' "${work_dir}/manifest.headers")

if [ "${actual_image_digest}" != "${IMAGE_DIGEST}" ]; then
  printf 'unexpected image digest: expected %s, got %s\n' \
    "${IMAGE_DIGEST}" "${actual_image_digest}" >&2
  exit 1
fi

config_digest=$(jq -r '.config.digest' "${work_dir}/manifest.json")
curl --fail --silent --show-error --insecure \
  --output "${work_dir}/config.json" \
  "${NEXUS_BASE_URL}/v2/${IMAGE_NAME}/blobs/${config_digest}"

platform=$(jq -r '.os + "/" + .architecture' "${work_dir}/config.json")
if [ "${platform}" != "linux/amd64" ]; then
  printf 'unexpected image platform: expected linux/amd64, got %s\n' "${platform}" >&2
  exit 1
fi

helm pull kube-blackbox \
  --repo "${HELM_REPOSITORY_URL}" \
  --version "${CHART_VERSION}" \
  --insecure-skip-tls-verify \
  --destination "${work_dir}"

chart_archive=${work_dir}/kube-blackbox-${CHART_VERSION}.tgz
actual_chart_digest=$(shasum -a 256 "${chart_archive}" | awk '{print $1}')
if [ "${actual_chart_digest}" != "${CHART_DIGEST}" ]; then
  printf 'unexpected chart digest: expected %s, got %s\n' \
    "${CHART_DIGEST}" "${actual_chart_digest}" >&2
  exit 1
fi

helm lint "${chart_archive}" >/dev/null
helm template kube-blackbox "${chart_archive}" \
  --namespace kube-blackbox \
  --values "${repository_root}/deploy/shturval-values.yaml" \
  > "${work_dir}/rendered.yaml"

if ! grep -Fq "mirror.ip-10-28-32-189.shturval.link/${IMAGE_NAME}@${IMAGE_DIGEST}" "${work_dir}/rendered.yaml"; then
  printf 'rendered chart does not reference the verified image digest\n' >&2
  exit 1
fi

printf 'image: %s/%s:%s (%s, %s)\n' \
  "${NEXUS_BASE_URL}" "${IMAGE_NAME}" "${IMAGE_TAG}" "${IMAGE_DIGEST}" "${platform}"
printf 'chart: %skube-blackbox-%s.tgz (sha256:%s)\n' \
  "${HELM_REPOSITORY_URL}" "${CHART_VERSION}" "${CHART_DIGEST}"
printf 'render: OK; Kubernetes API was not contacted\n'
