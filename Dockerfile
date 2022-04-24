FROM python
RUN mkdir /git
WORKDIR /git
RUN pip install git-dumper