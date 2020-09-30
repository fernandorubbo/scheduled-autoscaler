# Saving money by scaling down your Google Kubernetes Engine cluster at night

Authors:

- Fernando Rubbo | Cloud Solutions Architect | Google
- Joe Burnett | Software Engineer | Google

This tutorial explains how to deploy a scheduled autoscaler on Google Kubernetes Engine (GKE). This kind of autoscaler is very useful if your traffic usage increases abruptly in a predictive way. For example, if you are a regional retailer or if your software targets employees starting early in the morning and stopping at evening. This tutorial is for developers and operators who are looking for making application reliably scale before spikes arive, and willing to save money at night while users are sleeping.

<walkthrough-alt>

If you like, you can take the interactive version of this tutorial, which runs
in the Cloud Console:

[![Open in Cloud Console](https://walkthroughs.googleusercontent.com/tutorial/resources/open-in-console-button.svg)](https://ssh.cloud.google.com/cloudshell/editor?cloudshell_git_repo=https%3A%2F%2Fgithub.com%2Ffernandorubbo%2Fscheduled-autoscaler.git&cloudshell_working_dir=.&cloudshell_tutorial=tutorial.md)

</walkthrough-alt>

## Introduction

The majority of systems have its users engaging with the app during the day time, what makes most of data center servers idle at night. Beyond other benefits, the public cloud helps to save money by dynamically allocating the needed infrastructure according to the traffic load. There are cases where a simple autoscale configuration solves such allocation problem. However, in other cases big traffic spikes require a more fine tunning of the autoscale configurations to avoid system instability during scale ups.

This tutorial focus on scenarios where these traffic patterns are well known and you want to give the autoscaler some hints upfront spikes reache your infrastructure. Although this paper discusses GKE cluster scaling up in the morning and scaling down at night, you can use a similar approach to spin up capacity before known events, such as black friday, syber monday, tv comertials, etc.

![Scheduled Autoscaler](https://github.com/fernandorubbo/scheduled-autoscaler/blob/master/images/scheduled-autoscaler-arquitecture.png?raw=true)

The above picture shows how the scheduled autoscaler works. First a set of [CronJobs](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/) save the known traffic patterns into a [Cloud Monitoring custom metric](https://cloud.google.com/monitoring/custom-metrics). This data is then used by your [Horizontal Pod Autoscaler](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/) (HPA) as a on when HPA should preemptively scale your workload. Along with other load metrics, such as target CPU utilization, HPA decides how to update the replicas of a given deployment.

## Objectives

- Create a GKE cluster.
- Install the example application.
- Understand when scaling your cluster down at night is econnomically viable.
- Set up a scheduled autoscaler.
- Understand how HPA respond to either increase in traffic and custom metrics.

## Costs

This tutorial uses the following billable components of Google Cloud:

- [Cloud Monitoring](https://cloud.google.com/monitoring/pricing)
- [Container Registry](https://cloud.google.com/container-registry/pricing)
- [Google Kubernetes Engine](https://cloud.google.com/kubernetes-engine/pricing)

To generate a cost estimate based on your projected usage, use the
[pricing calculator](https://cloud.google.com/products/calculator).
New Google Cloud users might be eligible for a free trial.

When you finish this tutorial, you can avoid continued billing by deleting the
resources you created. For more information, see [Cleaning up](#cleaning-up).

## Before you begin

<!-- {% setvar project_id "YOUR_PROJECT_ID" %} -->

1. <walkthrough-project-billing-setup></walkthrough-project-billing-setup>

   <walkthrough-alt>

   [Sign in](https://accounts.google.com/Login) to your Google Account.

   If you don't already have one, [sign up for a new account](https://accounts.google.com/SignUp).

1. In the Cloud Console, on the project selector page, create a Cloud project.

    [Go to the project selector page](https://console.cloud.google.com/projectselector2/home/dashboard)

1. Make sure that billing is enabled for your Google Cloud project.
    [Learn how to confirm billing is enabled for your project.](https://cloud.google.com/billing/docs/how-to/modify-project)

1. In the Cloud Console, go to Cloud Shell and clone the repository containing
    the sample code.

    [Go to Cloud Shell](https://ssh.cloud.google.com/cloudshell/editor?cloudshell_git_repo=https%3A%2F%2Fgithub.com%2FGoogleCloudPlatform%2Fcommunity.git&cloudshell_working_dir=tutorials%2Fclaims-routing-istio)

    At the bottom of the screen, click **Confirm** to clone the Git repo into
    your Cloud Shell.

    At the bottom of the Cloud Console, a Cloud Shell session opens and
    displays a command-line prompt. Cloud Shell is a shell environment with the
    Cloud SDK already installed, including the `gcloud` command-line tool, and
    with values already set for your current project. It can take a few seconds
    for the session to initialize. You use Cloud Shell to run all the commands
    in this tutorial.

</walkthrough-alt>

1. In Cloud Shell, set the Google Cloud project you want to use for this
    tutorial and define your email address:

    ```bash
    PROJECT_ID={{project-id}}
    ALERT_EMAIL=<YOUR_EMAIL_ADDRESS>
    gcloud config set core/project $PROJECT_ID
    ```

1. Enable the GKE and Monitoring APIs:

    ```bash
    gcloud services enable \
        container.googleapis.com \
        monitoring.googleapis.com
    ```

1. Define `gcloud` command-line tool default for the Compute Engine region and
    zone that you want to use for this tutorial:

    ```bash
    gcloud config set compute/region us-central1
    gcloud config set compute/zone us-central1-f
    ```

    You can
    [choose a different region and zone](https://cloud.google.com/compute/docs/regions-zones)
    for this tutorial if you like.

## Creating the GKE cluster

1. Create a GKE cluster for running your scheduled autoscaler:

    ```bash
    gcloud beta container clusters create scheduled-autoscaler \
        --enable-ip-alias \
        --cluster-version=1.17 \
        --machine-type=e2-standard-2 \
        --enable-autoscaling --min-nodes=1 --max-nodes=10 \
        --num-nodes=1 \
        --autoscaling-profile=optimize-utilization
    ```

    The output should be similar to below:

    ```output
    NAME                   LOCATION       MASTER_VERSION   MASTER_IP      MACHINE_TYPE   NODE_VERSION     NUM_NODES  STATUS
    scheduled-autoscaler   us-central1-f  1.17.9-gke.6300  34.69.187.253  e2-standard-2  1.17.9-gke.6300  1          RUNNING
    ```

    **Note** This is not a production configuration, but useful for demostration. In the above setup, you configure [Cluster Autoscaler](https://cloud.google.com/kubernetes-engine/docs/concepts/cluster-autoscaler) with minimum 1 and maximum 10 nodes, and you enable [optimize-utilization](https://cloud.google.com/kubernetes-engine/docs/concepts/cluster-autoscaler#autoscaling_profiles) profile to speed up scale downs.

## Installing the example application

1. Deploy the example application without the scheduled autoscaler.

    ```bash
    kubectl apply -f ./k8s
    ```

1. Understand *k8s/hpa-example.yaml*.

    ```yaml
    spec:
        maxReplicas: 20
        minReplicas: 10
        scaleTargetRef:
            apiVersion: apps/v1
            kind: Deployment
            name: php-apache
        metrics:
            - type: Resource
                resource:
                    name: cpu
                    target:
                        type: Utilization
                        averageUtilization: 60
    ```

    Note that HPA has a minimum of replicas configured to *10* and it is configured to scale based on *CPU* utilization.

1. Wait the service to be become available.

    ```bash
    kubectl wait --for=condition=available --timeout=600s deployment/php-apache
    EXTERNAL_IP=''
    while [ -z $EXTERNAL_IP ]
    do EXTERNAL_IP=$(kubectl get svc php-apache -o jsonpath={.status.loadBalancer.ingress[0].ip}) && [ -z $EXTERNAL_IP ] && sleep 10
    done
    curl  http://$EXTERNAL_IP
    ```

1. Check the number of nodes and HPA replicas.

    ```bash
    kubectl get nodes
    kubectl get hpa php-apache
    ```

    The output should be similar to below:

    ```output
    NAME                                                  STATUS   ROLES    AGE   VERSION
    gke-scheduled-autoscaler-default-pool-64c02c0b-9kbt   Ready    <none>   21S   v1.17.9-gke.1504
    gke-scheduled-autoscaler-default-pool-64c02c0b-ghfr   Ready    <none>   21s   v1.17.9-gke.1504
    gke-scheduled-autoscaler-default-pool-64c02c0b-gvl9   Ready    <none>   21s   v1.17.9-gke.1504
    gke-scheduled-autoscaler-default-pool-64c02c0b-t9sr   Ready    <none>   21s   v1.17.9-gke.1504
    NAME         REFERENCE               TARGETS   MINPODS   MAXPODS   REPLICAS   AGE
    php-apache   Deployment/php-apache   9%/60%    10        20        10         6d19h
    ```

    Note that even that your cluster is set to have the minimum of 1 node, your workload is requesting more infrastructure by setting HPA *minReplicas* to 10. This is a very common strategy used by companies, such as retailers, which expect a sudden increase in traffic in the first business hours of the day. While setting high values for HPA *minReplicas* is a simple way of dealing with such scenarios, it can increse your costs because your cluster is unable shrink in size, not even at night while your users are sleeping.

## Committing to use discounts

If you want to scale down you cluster at night, you must first understand the basics of [committed-use discount](https://cloud.google.com/docs/cuds) (CUD). If you intend to stay with Google Cloud for a few years, we strongly recommend that you purchase committed-use discounts in return for deeply discounted prices for VM usage (up to 70% discount). If you are unsure about how much resource to commit, look at your minimum computing usage — for example, during night time — and commit the payment for that amount.

![Committed-use discount](https://github.com/fernandorubbo/scheduled-autoscaler/blob/master/images/commit-use.png?raw=true)

As you can see in the image above, CUD is flat. Meaning, the resource used during the day doen't compensate the unused resource during the night. Because of this reason, resources used by spikes should not be included in CUD. To optimize your costs, you must use GKE autoscaler options. For example, the scheduled autoscaler discussed in this paper or other managed options discussed in [Best practices for running cost-optimized Kubernetes applications on GKE](https://cloud.google.com/solutions/best-practices-for-running-cost-effective-kubernetes-applications-on-gke#fine-tune_gke_autoscaling). If you are already paying  CUD for a given amount of resources, there is no reason for not using it at night. In this case, try to schedule some jobs to fill the gaps of low computing demand, or keep your cluster warm even without usage.

## Setting up a scheduled autoscaler

After learning the basics of CUD, you should be capable of realizing if it econnomically viable to scale your cluster down at night or commit to use discount. This section teaches you how to use scheduled autoscaler, but keep in mind this is only useful if you don't sign a CUD or if your CUD doesn't leave idle resources during the night.

1. Install [stackdriver adapter](https://github.com/GoogleCloudPlatform/k8s-stackdriver/tree/master/custom-metrics-stackdriver-adapter).

    ```bash
    kubectl apply -f https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-stackdriver/master/custom-metrics-stackdriver-adapter/deploy/production/adapter_new_resource_model.yaml
    kubectl wait --for=condition=available --timeout=600s deployment/custom-metrics-stackdriver-adapter -n custom-metrics
    ```

1. Build and deploy your scheduled autoscaler.

    ```bash
    docker build -t gcr.io/$PROJECT_ID/custom-metric-extporter .
    docker push gcr.io/$PROJECT_ID/custom-metric-extporter
    sed -i.bak s/PROJECT_ID/$PROJECT_ID/g ./k8s/scheduled-autoscaler/scheduled-autoscale-example.yaml
    kubectl apply -f ./k8s/scheduled-autoscaler
    ```

1. Understand *k8s/scheduled-autoscaler/scheduled-autoscale-example.yaml* file.

    ```yaml
    apiVersion: batch/v1beta1
    kind: CronJob
    metadata:
        name: scale-up
    spec:
        schedule: "50-59/1 * * * *"
        jobTemplate:
            spec:
            template:
                spec:
                containers:
                - name: custom-metric-extporter
                    image: gcr.io/PROJECT_ID/custom-metric-extporter
                    command:
                    - /export
                    - --name=scheduled_autoscaler_example
                    - --value=10
    ---
    apiVersion: batch/v1beta1
    kind: CronJob
    metadata:
    name: scale-down
    spec:
    schedule: "1-49/1 * * * *"
    jobTemplate:
        spec:
        template:
            spec:
            containers:
            - name: custom-metric-extporter
                image: gcr.io/PROJECT_ID/custom-metric-extporter
                command:
                - /export
                - --name=scheduled_autoscaler_example
                - --value=1
            restartPolicy: OnFailure
    ```

    The *CronJobs* are sending the suggested Pod replicas count to a custom metric called **custom.googleapis.com/scheduled_autoscaler_example** based on the time of the day. To facilitate the monitoring section of this tutorial, the *schedule* field configuration define houly scale ups and downs. But you can customize it for a daily strategy pattern, or whatever matches your business needs.

1. Undestand *k8s/scheduled-autoscaler/hpa-example.yaml*.

    ```yaml
    spec:
        maxReplicas: 20
        minReplicas: 1
        scaleTargetRef:
            apiVersion: apps/v1
            kind: Deployment
            name: php-apache
        metrics:
            - type: Resource
                resource:
                    name: cpu
                    target:
                        type: Utilization
                        averageUtilization: 60
            - type: External
                external:
                    metric:
                        name: custom.googleapis.com|scheduled_autoscaler_example
                    target:
                        type: AverageValue
                        averageValue: 1

    ```

    This HPA object replaces the previously one already deployed. Note that it reduces the number of *minReplicas* to 1, so that the workload can be scaled down to the minimum, and adds an [*External* metric](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale-walkthrough/#autoscaling-on-multiple-metrics-and-custom-metrics), so that two factors must be considered to trigger an autoscaling activity. In this multiple metrics scenario, HPA will calculate proposed replicas count for each metric, and then choose the one with the highest value. This is extreanmly important to undestand because your schedule autoscaler can propose that in a given moment of time the Pod count should be 1, but if the actual usage of the CPU is higher than expected for 1 pod, HPA will spin up more replicas anyways.

1. Check the number of nodes and HPA replicas again.

    ```bash
    kubectl get nodes
    kubectl get hpa php-apache
    ```

    Around minutes 51-60 of any hour of the day, HPA min replicas must be 10 and nodes must be 4. Around minutes 1-50, HPA minReplicas must be 1 and nodes must be either 1 or 2, depending how pods were alocated and terminated. But note that for the later, your may have to wait around 10 min to your cluster finish to scale down.

## Alerting when scheduled autoscaler is not working properly

If you intent is to run such scheduled autoscaler in a production environment you probably want to be aware when your CronJobs are not populating the custom metric accordingly. For that you must create an alert that triggers when any *custom.googleapis.com/scheduled_autoscaler_example* stream is absent for greater than X minutes. Below steps guide you through this process.

1. In the console, click in *Monitoring* in the main menu.

1. Create a new workspace in the *Add your project to a Workspace* page by selecting your project and clicking in *ADD* button.

    Wait workspace creation to finish. This can take a minute or so to complete.

1. In Cloud Shell, create a notification channel.

    ```bash
    gcloud alpha monitoring channels create \
                --display-name="Scheduled Autoscaler team (Primary)" \
                --description="Primary contact method for the Scheduled Autoscaler team lead"  \
                --type=email \
                --channel-labels=email_address=$ALERT_EMAIL}
    ```

    The output should be similar to below:

    ```output
    Created notification channel [NOTIFICATION_CHANNEL].
    ```

    **Important**: we have created a notification channel of the type email to simplify the tutorial steps. However, in production environments, we strongly encorage you to use a less assynchronous strategy, such as instant message options.

1. Set a variable with the above value found in **NOTIFICATION_CHANNEL**.

    ```bash
    NOTIFICATION_CHANNEL=<NOTIFICATION_CHANNEL>
    ```

1. Deploy your alert policy.

    ```bash
    gcloud alpha monitoring policies create --policy-from-file=./monitoring/alert-policy.yaml --notification-channels=$NOTIFICATION_CHANNEL
    ```

1. Go to Cloud Monitoring Alerting to se the just created alert policy.

    [Go to Alerting page](https://console.cloud.google.com/monitoring/alerting).

1. Click in **Scheduled Autoscaler Policy** to see the details.

## Generating some load to your example application

1. In Cloud Shell, deploy your load generator.

    ```bash
    kubectl apply -f ./k8s/load-generator
    ```

1. Understand */k8s/load-generator/load-generator.yaml*.

    ```yaml
    command: ["/bin/sh", "-c"]
    args:
    - while true; do
        RESP=$(wget -q -O- http://php-apache.default.svc.cluster.local);
        echo "$(date +%H)=$RESP";
        sleep $(date +%H | awk '{ print "s("$0"/3*a(1))*0.5+0.5" }' | bc -l);
      done;
    ```

    This loop will keep running in your cluster until you delete the *load-generator* deployment. It make requests to your *php-apache* service every few milesseconds. The sleep function ensures load distribution changes during the day. This way you can better understand what happens when you combine CPU utilization and custom metrics in your HPA configuration.

## Understanding how the above setup respond to either increase in traffic or your scheduled metric

1. Create a new dashboard.

    ```bash
    gcloud monitoring dashboards create --config-from-file=./monitoring/dashboard.yaml
    ```

1. Go to the Cloud Monitoring Dashboards.

    [Go to the Dashboards page](https://console.cloud.google.com/monitoring/dashboards)

1. Click in **Scheduled Autoscaler Dashboard** to see the details.

    In such dashboard you find three graphs. However, you need to wait a period of 2 hours (ideally 24 hours) to see the dinamics of *scale-ups* and *scale-downs*, and how the different load distribution during the day impact autoscaling. To avoiding making you to wait such amount of time, the graphs below present a full day view.

    - **Scheduled Metric (desired # of Pods)** graph shows a time series of the custom metric you are sending to Cloud Monitoring through your CronJobs configured in [Setting up a scheduled autoscaler](#setting-up-a-scheduled-autoscaler). 

        ![Scheduled Metric (desired # of Pods)](https://github.com/fernandorubbo/scheduled-autoscaler/blob/master/images/scheduled-metric.png?raw=true)

    - **CPU Utilization (requested vs used)** graph shows a time series of CPU requested (red) vs the actual CPU usage. Note that when the load is low, HPA honors the scheduled autoscaler decision. However, when the traffic increases, it increases the number of Pods as needed (see data points between 12PM to 6PM).

        ![CPU Utilization (requested vs used)](https://github.com/fernandorubbo/scheduled-autoscaler/blob/master/images/cpu-utilization.png?raw=true)

    - **Number of Pods (scheduled vs actual) + Mean CPU Utilization** graph shows a similar view of the above ones. The Pod count (red) increases to 10 every hour on schedule (blue), load naturally increases and decreases pod count over time, and mean CPU utilization (orange) remains below the target (ie. 60%).

        ![Number of Pods (scheduled vs actual) + Mean CPU Utilization](https://github.com/fernandorubbo/scheduled-autoscaler/blob/master/images/pods-vs-mean-cpu.png?raw=true)

## Cleaning up

To avoid incurring continuing charges to your Google Cloud Platform account for
the resources used in this tutorial you can either delete the project or delete
the individual resources.

### Deleting the project

**Caution:**  Deleting a project has the following effects:

- **Everything in the project is deleted.** If you used an existing project
    for this tutorial, when you delete it, you also delete any other work
    you've done in the project.
- **Custom project IDs are lost.** When you created this project, you might
    have created a custom project ID that you want to use in the future. To
    preserve the URLs that use the project ID, such as an `appspot.com` URL,
    delete selected resources inside the project instead of deleting the whole
    project.

In Cloud Shell, run this command to delete the project:

```bash
echo $GOOGLE_CLOUD_PROJECT
gcloud projects delete $GOOGLE_CLOUD_PROJECT
```

## What's next

- Find more about GKE cost optimization in [Best practices for running cost-optimized Kubernetes applications on GKE](https://cloud.google.com/solutions/best-practices-for-running-cost-effective-kubernetes-applications-on-gke).
- Read [Running web applications on GKE using cost-optimized Preemptible VMs](https://cloud.google.com/solutions/running-web-applications-on-gke-using-cost-optimized-pvms-and-traffic-director).
- Find more tips and best practices for optimizing costs at [Cost optimization on Google Cloud for developers and operators](https://cloud.google.com/solutions/cost-efficiency-on-google-cloud).
- Read [Optimizing resource usage in a multi-tenant GKE cluster using node auto-provisioning](https://cloud.google.com/solutions/optimizing-resources-in-multi-tenant-gke-clusters-with-auto-provisioning) for more details on how to lower costs on batch applications.
- Try out other Google Cloud features for yourself. Have a look at our [tutorials](https://cloud.google.com/docs/tutorials).
