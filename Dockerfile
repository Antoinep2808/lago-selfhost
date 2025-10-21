FROM getlago/api:v1.33.2

EXPOSE 3000

ENV RAILS_ENV=production
ENV RACK_ENV=production
ENV LAGO_ENV=production
ENV WEB_CONCURRENCY=1

# Patch Puma config (désactive .present? check)
RUN sed -i "s/if ENV$begin:math:display$'WEB_CONCURRENCY'$end:math:display$.present?/if ENV['WEB_CONCURRENCY']/" config/puma.rb

# Démarrage normal
CMD ["bash", "-lc", "bundle exec puma -C config/puma.rb"]