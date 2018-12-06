#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

cd "$(dirname "$0")"

PROJECT=`gcloud config get-value project`
PROJECT_NUMBER=`gcloud projects describe ${PROJECT} --format='value(projectNumber)'`

SA_NAME=dns-stress-test

EMAIL=${SA_NAME}@${PROJECT}.iam.gserviceaccount.com

if [[ ! -f key.json ]]; then
    echo "key.json not found, creating service account and key"

    gcloud services enable cloudapis.googleapis.com


    gcloud iam service-accounts create $SA_NAME --display-name "Account for DNS stress tests"

    gcloud iam service-accounts keys create ./key.json --iam-account ${EMAIL}
else
    echo "found key.json, skipping service account creation"
fi


gcloud projects add-iam-policy-binding ${PROJECT} --member="serviceAccount:${EMAIL}" --role="roles/monitoring.metricWriter"
gcloud projects add-iam-policy-binding ${PROJECT} --member="serviceAccount:${EMAIL}" --role="roles/cloudtrace.agent"
