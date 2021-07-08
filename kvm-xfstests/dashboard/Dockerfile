FROM google/cloud-sdk

MAINTAINER Harshad Shirwakdar harshadshirwadkar@gmail.com

# Allow statements and log messages to immediately appear in the Knative logs
ENV PYTHONUNBUFFERED True

# Copy local code to the container image.
ENV APP_HOME /app
WORKDIR $APP_HOME
COPY . ./

# Install production dependencies.
RUN pip install Flask gunicorn junit-xml junitparser

# Install git
RUN apt-get install git jq -y

# Setup gce-xfstests inside the ocntainer
RUN mkdir -p $APP_HOME/xfstest-bld
RUN git clone https://github.com/tytso/xfstests-bld.git $APP_HOME/xfstests-bld
RUN cd $APP_HOME/xfstests-bld && make gce-xfstests.sh && cp gce-xfstests.sh /sbin/gce-xfstests

# Run the web service on container startup. Here we use the gunicorn
# webserver, with one worker process and 8 threads. For environments
# with multiple CPU cores, increase the number of workers to be equal
# to the cores available. Timeout is set to 0 to disable the timeouts
# of the workers to allow Cloud Run to handle instance scaling.
CMD exec gunicorn --bind :$PORT --workers 1 --threads 8 --timeout 0 dashboard:app
